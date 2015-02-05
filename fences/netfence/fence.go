package netfence

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry-incubator/garden-linux/fences"
	"github.com/cloudfoundry-incubator/garden-linux/fences/netfence/network/subnets"
	"github.com/cloudfoundry-incubator/garden-linux/old/sysconfig"
	"github.com/cloudfoundry-incubator/garden-linux/process"
	"github.com/pivotal-golang/lager"
)

type fenceBuilder struct {
	subnets.BridgedSubnets
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
// The given fence builder is stored in the returned fence.
func (f *fenceBuilder) Build(spec string, sysconfig *sysconfig.Config, containerID string) (fences.Fence, error) {
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

	subnet, containerIP, bridgeIfcName, err := f.BridgedSubnets.Allocate(subnetSelector, ipSelector)
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

	ones, _ := subnet.Mask.Size()
	subnetShareable := (ones < 30)

	return &Fence{
			IPNet:           subnet,
			containerIP:     containerIP,
			containerIfc:    containerIfcName,
			hostIfc:         hostIfcName,
			subnetShareable: subnetShareable,
			bridgeIfc:       bridgeIfcName,
			fenceBldr:       f,
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
func (f *fenceBuilder) Rebuild(rm *json.RawMessage) (fences.Fence, error) {
	ff := FlatFence{}
	if err := json.Unmarshal(*rm, &ff); err != nil {
		return nil, err
	}

	_, ipn, err := net.ParseCIDR(ff.Ipn)
	if err != nil {
		return nil, err
	}

	if err := f.BridgedSubnets.Recover(ipn, net.ParseIP(ff.ContainerIP), ff.BridgeIfcName); err != nil {
		return nil, err
	}

	containerIP := net.ParseIP(ff.ContainerIP)
	return &Fence{
		IPNet:           ipn,
		containerIP:     containerIP,
		containerIfc:    ff.ContainerIfcName,
		hostIfc:         ff.HostIfcName,
		subnetShareable: ff.SubnetShareable,
		bridgeIfc:       ff.BridgeIfcName,
		fenceBldr:       f,
		log:             f.log.Session("allocation", lager.Data{"subnet": ipn, "containerIP": containerIP}),
	}, nil
}

type Fence struct {
	*net.IPNet
	containerIP     net.IP
	containerIfc    string
	hostIfc         string
	subnetShareable bool
	bridgeIfc       string
	fenceBldr       *fenceBuilder
	log             lager.Logger
}

func (a *Fence) String() string {
	return fmt.Sprintf("%#v", *a)
}

func (a *Fence) Dismantle() error {
	subnetDeallocated, bridgeIfcName, err := a.fenceBldr.Release(a.IPNet, a.containerIP)
	if err != nil {
		return err
	}

	if subnetDeallocated {
		return a.fenceBldr.deconfigurer.DeconfigureBridge(a.log.Session("deconfigure-bridge"), bridgeIfcName)
	}

	return nil
}

func (a *Fence) Info(i *garden.ContainerInfo) {
	i.HostIP = subnets.GatewayIP(a.IPNet).String()
	i.ContainerIP = a.containerIP.String()
	i.ExternalIP = a.fenceBldr.externalIP.String()
}

func (a *Fence) MarshalJSON() ([]byte, error) {
	ff := FlatFence{a.IPNet.String(), a.containerIP.String(), a.containerIfc, a.hostIfc, a.subnetShareable, a.bridgeIfc}
	return json.Marshal(ff)
}

func (a *Fence) ConfigureProcess(env process.Env) error {
	suff, _ := a.IPNet.Mask.Size()

	env["network_host_ip"] = subnets.GatewayIP(a.IPNet).String()
	env["network_container_ip"] = a.containerIP.String()
	env["network_cidr_suffix"] = strconv.Itoa(suff)
	env["container_iface_mtu"] = strconv.FormatUint(uint64(a.fenceBldr.mtu), 10)
	env["subnet_shareable"] = strconv.FormatBool(a.subnetShareable)
	env["network_cidr"] = a.IPNet.String()
	env["external_ip"] = a.fenceBldr.externalIP.String()
	env["bridge_iface"] = a.bridgeIfc

	return nil
}
