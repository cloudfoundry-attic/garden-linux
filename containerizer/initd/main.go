package main

import "github.com/cloudfoundry-incubator/garden-linux/containerizer"

func main() {
	containerizer := containerizer.Containerizer{}
	containerizer.Child()
}
