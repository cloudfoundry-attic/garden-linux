package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/cloudfoundry-incubator/cf-lager"
	"github.com/cloudfoundry-incubator/garden-linux/network"
	"github.com/pivotal-golang/lager"
)

const defaultMtuSize = 1500

func main() {
	var verbose bool
	flag.BoolVar(&verbose, "v", false, "announce parameters on entry")

	var target string
	flag.StringVar(&target, "target", "host", "the target to configure (container or host)")

	var hostIfcName string
	flag.StringVar(&hostIfcName, "hostIfcName", "", "the name of the host-side device to configure")

	var containerIfcName string
	flag.StringVar(&containerIfcName, "containerIfcName", "", "the name of the container-side device to configure")

	var bridgeIfcName string
	flag.StringVar(&bridgeIfcName, "bridgeIfcName", "", "the name of the subnet's bridge device to configure")

	subnet := CidrVar{}
	flag.Var(&subnet, "subnet", "the container's subnet")

	gatewayIP := IPVar{}
	flag.Var(&gatewayIP, "gatewayIP", "the gateway IP of the container's subnet")

	containerIP := IPVar{}
	flag.Var(&containerIP, "containerIP", "the IP of the container")

	var mtu MtuVar = defaultMtuSize
	flag.Var(&mtu, "mtu", "the MTU size of the container-side device")

	var containerPid int
	flag.IntVar(&containerPid, "containerPid", 0, "the PID of the container's init process")

	flag.Parse()

	log := cf_lager.New("container-net")

	log.Info("args", lager.Data{
		"target":           target,
		"hostIfcName":      hostIfcName,
		"containerIfcName": containerIfcName,
		"containerIP":      containerIP.IP,
		"gatewayIP":        gatewayIP.IP,
		"bridgeIfcName":    bridgeIfcName,
		"subnet":           subnet.IPNet,
		"containerPid":     containerPid,
		"mtu":              int(mtu),
	})

	c := network.NewConfigurer(log)

	switch target {
	case "host":
		if err := c.ConfigureHost(hostIfcName, containerIfcName, bridgeIfcName, containerPid, gatewayIP.IP, subnet.IPNet, int(mtu)); err != nil {
			fmt.Printf("container-net: configure host: error %v", err)
			os.Exit(3)
		}
	case "container":
		if err := c.ConfigureContainer(containerIfcName, containerIP.IP, gatewayIP.IP, subnet.IPNet, int(mtu)); err != nil {
			fmt.Printf("container-net: configure container: error %v", err)
			os.Exit(3)
		}
	default:
		fmt.Println("invalid target:", target)
		os.Exit(2)
	}
}
