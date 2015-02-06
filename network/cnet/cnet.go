package cnet

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry-incubator/garden-linux/network/subnets"
	"github.com/cloudfoundry-incubator/garden-linux/process"
	"github.com/pivotal-golang/lager"
)

type ContainerNetwork interface {
	json.Marshaler
	ConfigureEnvironment(process.Env) error
	Info(*garden.ContainerInfo)
	String() string
}

// This should not be used outside this package.
type FlatContainerNetwork struct {
	Ipn              string
	ContainerIP      string
	ContainerIfcName string
	HostIfcName      string
	BridgeIfcName    string
}

type containerNetwork struct {
	ipNet        *net.IPNet
	containerIP  net.IP
	containerIfc string
	hostIfc      string
	bridgeIfc    string
	log          lager.Logger
}

func (cn *containerNetwork) String() string {
	return fmt.Sprintf("%#v", *cn)
}

func (cn *containerNetwork) Info(i *garden.ContainerInfo) {
	i.HostIP = subnets.GatewayIP(cn.ipNet).String()
	i.ContainerIP = cn.containerIP.String()
}

func (cn *containerNetwork) MarshalJSON() ([]byte, error) {
	fcn := FlatContainerNetwork{cn.ipNet.String(), cn.containerIP.String(), cn.containerIfc, cn.hostIfc, cn.bridgeIfc}
	return json.Marshal(fcn)
}

func (cn *containerNetwork) ConfigureEnvironment(env process.Env) error {
	suff, _ := cn.ipNet.Mask.Size()

	env["network_host_ip"] = subnets.GatewayIP(cn.ipNet).String()
	env["network_container_ip"] = cn.containerIP.String()
	env["network_cidr_suffix"] = strconv.Itoa(suff)
	env["network_cidr"] = cn.ipNet.String()
	env["bridge_iface"] = cn.bridgeIfc

	return nil
}
