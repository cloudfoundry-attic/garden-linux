package quota_manager

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry-incubator/garden-linux/old/logging"
	"github.com/cloudfoundry/gunk/command_runner"
	"github.com/pivotal-golang/lager"
)

type BtrfsQuotaManager struct {
	enabled bool
	runner  command_runner.CommandRunner
}

const QUOTA_BLOCK_SIZE = 1024

func New(runner command_runner.CommandRunner) *BtrfsQuotaManager {
	return &BtrfsQuotaManager{
		enabled: true,
		runner:  runner,
	}
}

func (m *BtrfsQuotaManager) Disable() {
	m.enabled = false
}

func (m *BtrfsQuotaManager) SetLimits(logger lager.Logger, subvolumePath string, limits garden.DiskLimits) error {
	if !m.enabled {
		return nil
	}

	runner := logging.Runner{
		Logger:        logger,
		CommandRunner: m.runner,
	}

	quotaInfo, err := m.quotaInfo(logger, subvolumePath)
	if err != nil {
		return err
	}

	cmd := exec.Command("btrfs", "qgroup", "limit", fmt.Sprintf("%d", limits.ByteHard), quotaInfo.Id, subvolumePath)
	if err := runner.Run(cmd); err != nil {
		return fmt.Errorf("quota_manager: failed to apply limit: %v", err)
	}

	return nil
}

func (m *BtrfsQuotaManager) GetLimits(logger lager.Logger, subvolumePath string) (garden.DiskLimits, error) {
	var limits garden.DiskLimits

	if !m.enabled {
		return limits, nil
	}

	quotaInfo, err := m.quotaInfo(logger, subvolumePath)
	if err != nil {
		return limits, err
	}

	limits.ByteHard = quotaInfo.Limit
	limits.ByteSoft = quotaInfo.Limit

	return limits, err
}

func (m *BtrfsQuotaManager) GetUsage(logger lager.Logger, subvolumePath string) (garden.ContainerDiskStat, error) {
	var (
		usage garden.ContainerDiskStat
		err   error
	)

	if !m.enabled {
		return usage, nil
	}

	quotaInfo, err := m.quotaInfo(logger, subvolumePath)
	if err != nil {
		return usage, err
	}

	usage.BytesUsed = quotaInfo.Usage

	return usage, nil
}

func (m *BtrfsQuotaManager) IsEnabled() bool {
	return m.enabled
}

type QuotaInfo struct {
	Id    string
	Usage uint64
	Limit uint64
}

func (m *BtrfsQuotaManager) quotaInfo(logger lager.Logger, path string) (*QuotaInfo, error) {
	var (
		cmdOut bytes.Buffer
		skip   int
		info   QuotaInfo
	)

	runner := logging.Runner{
		Logger:        logger,
		CommandRunner: m.runner,
	}

	cmd := exec.Command("sh", "-c", fmt.Sprintf("btrfs qgroup show -rF --raw %s", path))
	cmd.Stdout = &cmdOut

	if err := runner.Run(cmd); err != nil {
		return nil, fmt.Errorf("quota_manager: run quota info: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(cmdOut.String()), "\n")

	_, err := fmt.Sscanf(lines[len(lines)-1], "%s %d %d %d", &info.Id, &info.Usage, &skip, &info.Limit)
	if err != nil {
		return nil, fmt.Errorf("quota_manager: parse quota info: %v", err)
	}

	return &info, nil
}
