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

	var containerIfcName string
	flag.StringVar(&containerIfcName, "containerIfcName", "", "the name of the container-side device to configure")

	subnet := network.CidrVar{}
	flag.Var(&subnet, "subnet", "the container's subnet")

	gatewayIP := network.IPVar{}
	flag.Var(&gatewayIP, "gatewayIP", "the gateway IP of the container's subnet")

	containerIP := network.IPVar{}
	flag.Var(&containerIP, "containerIP", "the IP of the container")

	var mtu network.MtuVar = defaultMtuSize
	flag.Var(&mtu, "mtu", "the MTU size of the container-side device")

	flag.Parse()

	if verbose {
		fmt.Println("\nnet-fence:",
			"\n  containerIfcName", containerIfcName,
			"\n  containerIP", containerIP.IP,
			"\n  gatewayIP", gatewayIP.IP,
			"\n  subnet", subnet.IPNet,
			"\n  mtu", int(mtu),
		)
	}

	err := network.ConfigureContainer(containerIfcName, containerIP.IP, gatewayIP.IP, subnet.IPNet, int(mtu))
	if err != nil {
		fmt.Println("error:", err)
		os.Exit(1)
	}
}
