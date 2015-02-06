package cnet

import (
	"encoding/json"
	"errors"
	"net"
	"strconv"
	"strings"

	"github.com/cloudfoundry-incubator/garden-linux/network/subnets"
	"github.com/cloudfoundry-incubator/garden-linux/old/sysconfig"
	"github.com/cloudfoundry-incubator/garden-linux/process"
	"github.com/pivotal-golang/lager"
)

type Builder interface {
	Build(spec string, sysconfig *sysconfig.Config, containerID string) (ContainerNetwork, error)
	Rebuild(*json.RawMessage) (ContainerNetwork, error)
	Dismantle(cn ContainerNetwork) error
	Capacity() int
	ConfigureEnvironment(env process.Env) error
	ExternalIP() net.IP
}

type containerNetworkBuilder struct {
	bs           subnets.BridgedSubnets
	mtu          uint32
	externalIP   net.IP
	deconfigurer interface {
		DeconfigureBridge(logger lager.Logger, bridgeIfc string) error
	}

	log lager.Logger
}

// Builds a container network from a given network spec. If the network spec
// is empty, dynamically allocates a subnet and IP. Otherwise, if the network
// spec specifies a subnet IP, allocates that subnet, and an available
// dynamic IP address. If the network has non-empty host bits, this exact IP
// address is statically allocated. In all cases, if an IP cannot be allocated which
// meets the requirements, an error is returned.
//
// The given container network builder is stored in the returned container network.
func (cnb *containerNetworkBuilder) Build(spec string, sysconfig *sysconfig.Config, containerID string) (ContainerNetwork, error) {
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

	subnet, containerIP, bridgeIfcName, err := cnb.bs.Allocate(subnetSelector, ipSelector)
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

	return &containerNetwork{
			ipNet:        subnet,
			containerIP:  containerIP,
			containerIfc: containerIfcName,
			hostIfc:      hostIfcName,
			bridgeIfc:    bridgeIfcName,
			log:          cnb.log.Session("allocation", lager.Data{"subnet": subnet, "ip": containerIP})},
		nil
}

// Rebuilds a container network from the marshalled JSON from an existing container network's MarshalJSON method.
// Returns an error if any of the allocations stored in the recovered container network are no longer
// available.
func (cnb *containerNetworkBuilder) Rebuild(rm *json.RawMessage) (ContainerNetwork, error) {
	fcn := FlatContainerNetwork{}
	if err := json.Unmarshal(*rm, &fcn); err != nil {
		return nil, err
	}

	_, ipn, err := net.ParseCIDR(fcn.Ipn)
	if err != nil {
		return nil, err
	}

	if err := cnb.bs.Recover(ipn, net.ParseIP(fcn.ContainerIP), fcn.BridgeIfcName); err != nil {
		return nil, err
	}

	containerIP := net.ParseIP(fcn.ContainerIP)
	return &containerNetwork{
		ipNet:        ipn,
		containerIP:  containerIP,
		containerIfc: fcn.ContainerIfcName,
		hostIfc:      fcn.HostIfcName,
		bridgeIfc:    fcn.BridgeIfcName,
		log:          cnb.log.Session("allocation", lager.Data{"subnet": ipn, "containerIP": containerIP}),
	}, nil
}

func (cnb *containerNetworkBuilder) Dismantle(ctrNetwork ContainerNetwork) error {
	cn, ok := ctrNetwork.(*containerNetwork)
	if !ok {
		return errors.New("ContainerNetwork has wrong concrete type")
	}
	subnetDeallocated, bridgeIfcName, err := cnb.bs.Release(cn.ipNet, cn.containerIP)
	if err != nil {
		return err
	}

	if subnetDeallocated {
		return cnb.deconfigurer.DeconfigureBridge(cnb.log.Session("deconfigure-bridge"), bridgeIfcName)
	}

	return nil
}

func (cnb *containerNetworkBuilder) Capacity() int {
	return cnb.bs.Capacity()
}

func (cnb *containerNetworkBuilder) ConfigureEnvironment(env process.Env) error {
	env["container_iface_mtu"] = strconv.FormatUint(uint64(cnb.mtu), 10)
	env["external_ip"] = cnb.externalIP.String()

	return nil
}

func (cnb *containerNetworkBuilder) ExternalIP() net.IP {
	return cnb.externalIP
}

func suffixIfNeeded(spec string) string {
	if !strings.Contains(spec, "/") {
		spec = spec + "/30"
	}

	return spec
}
