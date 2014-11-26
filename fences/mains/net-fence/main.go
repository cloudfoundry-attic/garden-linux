package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/cloudfoundry-incubator/garden-linux/fences/network"
)

const defaultMtuSize = 1500

func main() {
	var verbose bool
	flag.BoolVar(&verbose, "v", false, "announce parameters on entry")

	var target string
	flag.StringVar(&target, "target", "host", "the target to configure (container or host)")

	var tag string
	flag.StringVar(&tag, "tag", "99", "a tag to disambiguate resource names")

	var hostIfcName string
	flag.StringVar(&hostIfcName, "hostIfcName", "", "the name of the host-side device to configure")

	var containerIfcName string
	flag.StringVar(&containerIfcName, "containerIfcName", "", "the name of the container-side device to configure")

	var bridgeIfcName string
	flag.StringVar(&bridgeIfcName, "bridgeIfcName", "", "the name of the subnet's bridge device to configure")

	subnet := network.CidrVar{}
	flag.Var(&subnet, "subnet", "the container's subnet")

	gatewayIP := network.IPVar{}
	flag.Var(&gatewayIP, "gatewayIP", "the gateway IP of the container's subnet")

	containerIP := network.IPVar{}
	flag.Var(&containerIP, "containerIP", "the IP of the container")

	var mtu network.MtuVar = defaultMtuSize
	flag.Var(&mtu, "mtu", "the MTU size of the container-side device")

	var containerPid int
	flag.IntVar(&containerPid, "containerPid", 0, "the PID of the container's init process")

	flag.Parse()

	if verbose {
		fmt.Println("\nnet-fence:",
			"\n  target", target,
			"\n  tag", tag,
			"\n  hostIfcName", hostIfcName,
			"\n  containerIfcName", containerIfcName,
			"\n  containerIP", containerIP.IP,
			"\n  gatewayIP", gatewayIP.IP,
			"\n  bridgeIfcName", bridgeIfcName,
			"\n  subnet", subnet.IPNet,
			"\n  containerPid", containerPid,
			"\n  mtu", int(mtu),
		)
	}

	var err error
	if target == "host" {
		err = network.ConfigureHost(hostIfcName, containerIfcName, gatewayIP.IP, bridgeIfcName, subnet.IPNet, containerPid, int(mtu), tag)
	} else if target == "container" {
		err = network.ConfigureContainer(containerIfcName, containerIP.IP, gatewayIP.IP, subnet.IPNet, int(mtu))
	} else {
		fmt.Println("invalid target:", target)
		os.Exit(1)
	}
	if err != nil {
		fmt.Println("error:", err)
		os.Exit(1)
	}
}
