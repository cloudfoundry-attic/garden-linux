package quota_manager

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry-incubator/garden-linux/old/logging"
	"github.com/cloudfoundry/gunk/command_runner"
	"github.com/pivotal-golang/lager"
)

type QuotaManager interface {
	SetLimits(logger lager.Logger, uid int, limits garden.DiskLimits) error
	GetLimits(logger lager.Logger, uid int) (garden.DiskLimits, error)
	GetUsage(logger lager.Logger, uid int) (garden.ContainerDiskStat, error)

	MountPoint() string
	Disable()
	IsEnabled() bool
}

type LinuxQuotaManager struct {
	enabled bool

	binPath string
	runner  command_runner.CommandRunner

	mountPoint string
}

const QUOTA_BLOCK_SIZE = 1024

func New(runner command_runner.CommandRunner, mountPoint, binPath string) *LinuxQuotaManager {
	return &LinuxQuotaManager{
		enabled: true,

		binPath: binPath,
		runner:  runner,

		mountPoint: mountPoint,
	}
}

func (m *LinuxQuotaManager) Disable() {
	m.enabled = false
}

func (m *LinuxQuotaManager) SetLimits(logger lager.Logger, uid int, limits garden.DiskLimits) error {
	if !m.enabled {
		return nil
	}

	if limits.ByteSoft != 0 {
		limits.BlockSoft = (limits.ByteSoft + QUOTA_BLOCK_SIZE - 1) / QUOTA_BLOCK_SIZE
	}

	if limits.ByteHard != 0 {
		limits.BlockHard = (limits.ByteHard + QUOTA_BLOCK_SIZE - 1) / QUOTA_BLOCK_SIZE
	}

	runner := logging.Runner{
		Logger:        logger,
		CommandRunner: m.runner,
	}

	return runner.Run(
		exec.Command(
			"setquota",
			"-u",
			fmt.Sprintf("%d", uid),
			fmt.Sprintf("%d", limits.BlockSoft),
			fmt.Sprintf("%d", limits.BlockHard),
			fmt.Sprintf("%d", limits.InodeSoft),
			fmt.Sprintf("%d", limits.InodeHard),
			m.mountPoint,
		),
	)
}

func (m *LinuxQuotaManager) GetLimits(logger lager.Logger, uid int) (garden.DiskLimits, error) {
	if !m.enabled {
		return garden.DiskLimits{}, nil
	}

	repquota := exec.Command(path.Join(m.binPath, "repquota"), m.mountPoint, fmt.Sprintf("%d", uid))

	limits := garden.DiskLimits{}

	repR, repW, err := os.Pipe()
	if err != nil {
		return limits, err
	}

	defer repR.Close()
	defer repW.Close()

	repquota.Stdout = repW

	runner := logging.Runner{
		Logger:        logger,
		CommandRunner: m.runner,
	}

	err = runner.Start(repquota)
	if err != nil {
		return limits, err
	}

	defer runner.Wait(repquota)

	var skip uint32

	_, err = fmt.Fscanf(
		repR,
		"%d %d %d %d %d %d %d %d",
		&skip,
		&skip,
		&limits.BlockSoft,
		&limits.BlockHard,
		&skip,
		&skip,
		&limits.InodeSoft,
		&limits.InodeHard,
	)

	return limits, err
}

func (m *LinuxQuotaManager) GetUsage(logger lager.Logger, uid int) (garden.ContainerDiskStat, error) {
	if !m.enabled {
		return garden.ContainerDiskStat{}, nil
	}

	repquota := exec.Command(path.Join(m.binPath, "repquota"), m.mountPoint, fmt.Sprintf("%d", uid))

	usage := garden.ContainerDiskStat{}

	out := new(bytes.Buffer)

	repquota.Stdout = out

	runner := logging.Runner{
		Logger:        logger,
		CommandRunner: m.runner,
	}

	err := runner.Run(repquota)
	if err != nil {
		return usage, err
	}

	var skip uint32

	_, err = fmt.Fscanf(
		out,
		"%d %d %d %d %d %d %d %d",
		&skip,
		&usage.BytesUsed,
		&skip,
		&skip,
		&skip,
		&usage.InodesUsed,
		&skip,
		&skip,
	)

	return usage, err
}

func (m *LinuxQuotaManager) MountPoint() string {
	return m.mountPoint
}

func (m *LinuxQuotaManager) IsEnabled() bool {
	return m.enabled
}
