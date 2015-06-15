package linux_container

import (
	"fmt"
	"os/exec"
	"path"
	"strconv"

	"github.com/cloudfoundry-incubator/garden"
)

func (c *LinuxContainer) LimitBandwidth(limits garden.BandwidthLimits) error {
	cLog := c.logger.Session("limit-bandwidth")

	err := c.bandwidthManager.SetLimits(cLog, limits)
	if err != nil {
		return err
	}

	c.bandwidthMutex.Lock()
	defer c.bandwidthMutex.Unlock()

	c.LinuxContainerSpec.Limits.Bandwidth = &limits

	return nil
}

func (c *LinuxContainer) CurrentBandwidthLimits() (garden.BandwidthLimits, error) {
	c.bandwidthMutex.RLock()
	defer c.bandwidthMutex.RUnlock()

	if c.LinuxContainerSpec.Limits.Bandwidth == nil {
		return garden.BandwidthLimits{}, nil
	}

	return *c.LinuxContainerSpec.Limits.Bandwidth, nil
}

func (c *LinuxContainer) LimitDisk(limits garden.DiskLimits) error {
	cLog := c.logger.Session("limit-disk")

	err := c.quotaManager.SetLimits(cLog, c.RootFSPath(), limits)
	if err != nil {
		return err
	}

	c.diskMutex.Lock()
	defer c.diskMutex.Unlock()

	c.LinuxContainerSpec.Limits.Disk = &limits

	return nil
}

func (c *LinuxContainer) CurrentDiskLimits() (garden.DiskLimits, error) {
	cLog := c.logger.Session("current-disk-limits")
	return c.quotaManager.GetLimits(cLog, c.RootFSPath())
}

func (c *LinuxContainer) LimitMemory(limits garden.MemoryLimits) error {
	err := c.startOomNotifier()
	if err != nil {
		return err
	}

	limit := fmt.Sprintf("%d", limits.LimitInBytes)

	// memory.memsw.limit_in_bytes must be >= memory.limit_in_bytes
	//
	// however, it must be set after memory.limit_in_bytes, and if we're
	// increasing the limit, writing memory.limit_in_bytes first will fail.
	//
	// so, write memory.limit_in_bytes before and after
	c.cgroupsManager.Set("memory", "memory.limit_in_bytes", limit)
	c.cgroupsManager.Set("memory", "memory.memsw.limit_in_bytes", limit)

	err = c.cgroupsManager.Set("memory", "memory.limit_in_bytes", limit)
	if err != nil {
		return err
	}

	c.memoryMutex.Lock()
	defer c.memoryMutex.Unlock()

	c.LinuxContainerSpec.Limits.Memory = &limits

	return nil
}

func (c *LinuxContainer) CurrentMemoryLimits() (garden.MemoryLimits, error) {
	limitInBytes, err := c.cgroupsManager.Get("memory", "memory.limit_in_bytes")
	if err != nil {
		return garden.MemoryLimits{}, err
	}

	numericLimit, err := strconv.ParseUint(limitInBytes, 10, 0)
	if err != nil {
		return garden.MemoryLimits{}, err
	}

	return garden.MemoryLimits{uint64(numericLimit)}, nil
}

func (c *LinuxContainer) LimitCPU(limits garden.CPULimits) error {
	limit := fmt.Sprintf("%d", limits.LimitInShares)

	err := c.cgroupsManager.Set("cpu", "cpu.shares", limit)
	if err != nil {
		return err
	}

	c.cpuMutex.Lock()
	defer c.cpuMutex.Unlock()

	c.LinuxContainerSpec.Limits.CPU = &limits

	return nil
}

func (c *LinuxContainer) CurrentCPULimits() (garden.CPULimits, error) {
	actualLimitInShares, err := c.cgroupsManager.Get("cpu", "cpu.shares")
	if err != nil {
		return garden.CPULimits{}, err
	}

	numericLimit, err := strconv.ParseUint(actualLimitInShares, 10, 0)
	if err != nil {
		return garden.CPULimits{}, err
	}

	return garden.CPULimits{uint64(numericLimit)}, nil
}

func (c *LinuxContainer) startOomNotifier() error {
	c.oomMutex.Lock()
	defer c.oomMutex.Unlock()

	if c.oomNotifier != nil {
		return nil
	}

	oomPath := path.Join(c.ContainerPath, "bin", "oom")

	memorySubsystemPath, err := c.cgroupsManager.SubsystemPath("memory")
	if err != nil {
		return fmt.Errorf("linux_container: startOomNotifier: %s", err)
	}
	c.oomNotifier = exec.Command(oomPath, memorySubsystemPath)

	err = c.runner.Start(c.oomNotifier)
	if err != nil {
		return err
	}

	go c.watchForOom(c.oomNotifier)

	return nil
}

func (c *LinuxContainer) stopOomNotifier() {
	c.oomMutex.RLock()
	defer c.oomMutex.RUnlock()

	if c.oomNotifier != nil {
		c.runner.Kill(c.oomNotifier)
	}
}

func (c *LinuxContainer) watchForOom(oom *exec.Cmd) {
	err := c.runner.Wait(oom)
	if err == nil {
		c.registerEvent("out of memory")
		c.Stop(false)
	}

	// TODO: handle case where oom notifier itself failed? kill container?
}
