package container_pool

import "github.com/docker/docker/daemon/graphdriver"

//go:generate counterfeiter -o fake_graph_driver/fake_graph_driver.go . GraphDriver

type GraphDriver interface {
	graphdriver.Driver
}
