package quota_manager

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/pivotal-golang/lager"
)

//go:generate counterfeiter -o fake_aufsdiffpathfinder/FakeAUFSDiffPathFinder.go . AUFSDiffPathFinder
type AUFSDiffPathFinder interface {
	GetDiffLayerPath(rootFSPath string) string
}

type AUFSQuotaManager struct {
	AUFSDiffPathFinder AUFSDiffPathFinder
}

func (*AUFSQuotaManager) SetLimits(logger lager.Logger, containerRootFSPath string, limits garden.DiskLimits) error {
	return nil
}

func (*AUFSQuotaManager) GetLimits(logger lager.Logger, containerRootFSPath string) (garden.DiskLimits, error) {
	return garden.DiskLimits{}, nil
}

func (a *AUFSQuotaManager) GetUsage(logger lager.Logger, containerRootFSPath string) (garden.ContainerDiskStat, error) {
	_, err := os.Stat(containerRootFSPath)
	if os.IsNotExist(err) {
		return garden.ContainerDiskStat{}, fmt.Errorf("get usage: %s", err)
	}

	command := fmt.Sprintf("df -B 1 | grep %s | awk -v N=3 '{print $N}'", a.AUFSDiffPathFinder.GetDiffLayerPath((containerRootFSPath)))
	outbytes, err := exec.Command("sh", "-c", command).CombinedOutput()
	if err != nil {
		return garden.ContainerDiskStat{}, fmt.Errorf("get usage: df: %s, %s", err, string(outbytes))
	}

	var bytesUsed uint64
	if _, err := fmt.Sscanf(string(outbytes), "%d", &bytesUsed); err != nil {
		return garden.ContainerDiskStat{ExclusiveBytesUsed: 0}, nil
	}

	return garden.ContainerDiskStat{
		ExclusiveBytesUsed: bytesUsed,
	}, nil

	return garden.ContainerDiskStat{}, nil
}

func (*AUFSQuotaManager) Setup() error {
	return nil
}

func (*AUFSQuotaManager) IsEnabled() bool {
	return false
}
