package quota_manager

import (
	"fmt"
	"os/exec"
	"path/filepath"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/pivotal-golang/lager"
)

type AUFSQuotaManager struct{}

func (AUFSQuotaManager) SetLimits(logger lager.Logger, containerRootFSPath string, limits garden.DiskLimits) error {
	return nil
}

func (AUFSQuotaManager) GetLimits(logger lager.Logger, containerRootFSPath string) (garden.DiskLimits, error) {
	return garden.DiskLimits{}, nil
}

func (AUFSQuotaManager) GetUsage(logger lager.Logger, containerRootFSPath string) (garden.ContainerDiskStat, error) {
	fmt.Println("root path " + string(filepath.Base(containerRootFSPath)))
	command := fmt.Sprintf("df -B 1 | grep %s | awk -v N=3 '{print $N}'", filepath.Base(containerRootFSPath))
	fmt.Println(command)
	outbytes, _ := exec.Command("sh", "-c", command).Output()
	fmt.Println("Usage words " + string(outbytes))
	var bytesUsed uint64
	if _, err := fmt.Sscanf(string(outbytes), "%d", &bytesUsed); err != nil {
		panic("Argh!")
	}
	return garden.ContainerDiskStat{
		ExclusiveBytesUsed: bytesUsed,
	}, nil
}

func (AUFSQuotaManager) Setup() error {
	return nil
}

func (AUFSQuotaManager) IsEnabled() bool {
	return false
}
