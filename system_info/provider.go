package system_info

import (
	"github.com/cloudfoundry/gosigar"
)

type Provider interface {
	TotalMemory() (uint64, error)
	TotalDisk() (uint64, error)
}

type provider struct {
	depotPath string
}

func NewProvider(depotPath string) Provider {
	return &provider{
		depotPath: depotPath,
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

func fromKBytesToBytes(kbytes uint64) uint64 {
	return kbytes * 1024
}
