package sysinfo

import (
	"fmt"
	"os"

	"io/ioutil"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry/gosigar"
)

//go:generate counterfeiter -o fake_sysinfo/FakeProvider.go . Provider

type Provider interface {
	TotalMemory() (uint64, error)
	TotalDisk() (uint64, error)
	CheckHealth() error
}

type provider struct {
	depotPath string
	graphPath string
}

func NewProvider(depotPath string, graphPath string) Provider {
	return &provider{
		depotPath: depotPath,
		graphPath: graphPath,
	}
}

func (provider *provider) TotalMemory() (uint64, error) {
	mem := sigar.Mem{}

	err := mem.Get()
	if err != nil {
		return 0, err
	}

	return mem.Total, nil
}

func (provider *provider) TotalDisk() (uint64, error) {
	disk := sigar.FileSystemUsage{}

	err := disk.Get(provider.depotPath)
	if err != nil {
		return 0, err
	}

	return fromKBytesToBytes(disk.Total), nil
}

func (provider *provider) CheckHealth() error {
	f, err := ioutil.TempFile(provider.graphPath, "healthprobe")
	if err != nil {
		return garden.NewUnrecoverableError(fmt.Sprintf("graph directory '%s' is not writeable: %s", provider.graphPath, err))
	}

	f.Close()
	os.Remove(f.Name())

	return nil
}

func fromKBytesToBytes(kbytes uint64) uint64 {
	return kbytes * 1024
}
