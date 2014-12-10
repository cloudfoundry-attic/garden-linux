package netfence

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"strings"

	"github.com/cloudfoundry-incubator/garden-linux/fences"
	"github.com/cloudfoundry-incubator/garden-linux/fences/netfence/network/subnets"
	"github.com/cloudfoundry-incubator/garden-linux/old/sysconfig"
	"github.com/cloudfoundry-incubator/garden/api"
	"github.com/pivotal-golang/lager"
)

type f struct {
	subnets.Subnets
	mtu          uint32
	externalIP   net.IP
	deconfigurer interface {
		DeconfigureBridge(logger lager.Logger, bridgeIfc string) error
	}

	log lager.Logger
}

type FlatFence struct {
	Ipn              string
	ContainerIP      string
	ContainerIfcName string
	HostIfcName      string
	SubnetShareable  bool
	BridgeIfcName    string
}

// Builds a (network) Fence from a given network spec. If the network spec
// is empty, dynamically allocates a subnet and IP. Otherwise, if the network
// spec specifies a subnet IP, allocates that subnet, and an available
// dynamic IP address. If the network has non-empty host bits, this exact IP
// address is statically allocated. In all cases, if an IP cannot be allocated which
// meets the requirements, an error is returned.
//
// The given allocation is stored in the returned fence.
func (f *f) Build(spec string, sysconfig *sysconfig.Config, containerID string) (fences.Fence, error) {
	var ipSelector subnets.IPSelector = subnets.DynamicIPSelector
	var subnetSelector subnets.SubnetSelector = subnets.DynamicSubnetSelector

	if spec != "" {
		specifiedIP, ipn, err := net.ParseCIDR(suffixIfNeeded(spec))
		if err != nil {
			return nil, err
		}

		subnetSelector = subnets.StaticSubnetSelector{ipn}

		if !specifiedIP.Equal(subnets.NetworkIP(ipn)) {
			ipSelector = subnets.StaticIPSelector{specifiedIP}
		}
	}

	subnet, containerIP, err := f.Subnets.Allocate(subnetSelector, ipSelector)
	if err != nil {
		return nil, err
	}

	prefix := sysconfig.NetworkInterfacePrefix
	maxIdLen := 14 - len(prefix) // 14 is maximum interface name size - room for "-0"

	var ifaceName string
	if len(containerID) < maxIdLen {
		ifaceName = containerID
	} else {
		ifaceName = containerID[len(containerID)-maxIdLen:]
	}

	containerIfcName := prefix + ifaceName + "-1"
	hostIfcName := prefix + ifaceName + "-0"
	bridgeIfcName := prefix + "br-" + hexIP(subnet.IP)

	ones, _ := subnet.Mask.Size()
	subnetShareable := (ones < 30)

	return &Allocation{
			IPNet:           subnet,
			containerIP:     containerIP,
			containerIfc:    containerIfcName,
			hostIfc:         hostIfcName,
			subnetShareable: subnetShareable,
			bridgeIfc:       bridgeIfcName,
			fence:           f,
			log:             f.log.Session("allocation", lager.Data{"subnet": subnet, "ip": containerIP})},
		nil
}

func suffixIfNeeded(spec string) string {
	if !strings.Contains(spec, "/") {
		spec = spec + "/30"
	}

	return spec
}

// Rebuilds a Fence from the marshalled JSON from an existing Fence's MarshalJSON method.
// Returns an error if any of the allocations stored in the recovered fence are no longer
// available.
func (f *f) Rebuild(rm *json.RawMessage) (fences.Fence, error) {
	ff := FlatFence{}
	if err := json.Unmarshal(*rm, &ff); err != nil {
		return nil, err
	}

	_, ipn, err := net.ParseCIDR(ff.Ipn)
	if err != nil {
		return nil, err
	}

	if err := f.Subnets.Recover(ipn, net.ParseIP(ff.ContainerIP)); err != nil {
		return nil, err
	}

	containerIP := net.ParseIP(ff.ContainerIP)
	return &Allocation{
		IPNet:           ipn,
		containerIP:     containerIP,
		containerIfc:    ff.ContainerIfcName,
		hostIfc:         ff.HostIfcName,
		subnetShareable: ff.SubnetShareable,
		bridgeIfc:       ff.BridgeIfcName,
		fence:           f,
		log:             f.log.Session("allocation", lager.Data{"subnet": ipn, "containerIP": containerIP}),
	}, nil
}

type Allocation struct {
	*net.IPNet
	containerIP     net.IP
	containerIfc    string
	hostIfc         string
	subnetShareable bool
	bridgeIfc       string
	fence           *f
	log             lager.Logger
}

type Destroyer interface {
	Destroy() error
}

func (a *Allocation) String() string {
	return fmt.Sprintf("%#v", *a)
}

func (a *Allocation) Dismantle() error {
	releasedSubnet, err := a.fence.Release(a.IPNet, a.containerIP)
	if err != nil {
		return err
	}

	if releasedSubnet {
		return a.fence.deconfigurer.DeconfigureBridge(a.log.Session("deconfigure-bridge"), a.bridgeIfc)
	}

	return nil
}

func (a *Allocation) Info(i *api.ContainerInfo) {
	i.HostIP = subnets.GatewayIP(a.IPNet).String()
	i.ContainerIP = a.containerIP.String()
	i.ExternalIP = a.fence.externalIP.String()
}

func (a *Allocation) MarshalJSON() ([]byte, error) {
	ff := FlatFence{a.IPNet.String(), a.containerIP.String(), a.containerIfc, a.hostIfc, a.subnetShareable, a.bridgeIfc}
	return json.Marshal(ff)
}

func (a *Allocation) ConfigureProcess(env *[]string) error {
	suff, _ := a.IPNet.Mask.Size()

	*env = append(*env, fmt.Sprintf("network_host_ip=%s", subnets.GatewayIP(a.IPNet)),
		fmt.Sprintf("network_container_ip=%s", a.containerIP),
		fmt.Sprintf("network_cidr_suffix=%d", suff),
		fmt.Sprintf("container_iface_mtu=%d", a.fence.mtu),
		fmt.Sprintf("subnet_shareable=%v", a.subnetShareable),
		fmt.Sprintf("network_cidr=%s", a.IPNet.String()),
		fmt.Sprintf("external_ip=%s", a.fence.externalIP.String()),
		fmt.Sprintf("network_ip_hex=%s", hexIP(a.IPNet.IP))) // suitable for short bridge interface names

	return nil
}

func hexIP(ip net.IP) string {
	return hex.EncodeToString(ip)
}
