package linux_backend

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cloudfoundry-incubator/garden/warden"
	"github.com/cloudfoundry-incubator/warden-linux/linux_backend/bandwidth_manager"
	"github.com/cloudfoundry-incubator/warden-linux/linux_backend/cgroups_manager"
	"github.com/cloudfoundry-incubator/warden-linux/linux_backend/process_tracker"
	"github.com/cloudfoundry-incubator/warden-linux/linux_backend/quota_manager"
	"github.com/cloudfoundry/gunk/command_runner"
)

type LinuxContainer struct {
	id     string
	handle string
	path   string

	properties warden.Properties

	graceTime time.Duration

	state      State
	stateMutex sync.RWMutex

	events      []string
	eventsMutex sync.RWMutex

	resources *Resources

	portPool PortPool

	runner command_runner.CommandRunner

	cgroupsManager   cgroups_manager.CgroupsManager
	quotaManager     quota_manager.QuotaManager
	bandwidthManager bandwidth_manager.BandwidthManager

	processTracker process_tracker.ProcessTracker

	oomMutex    sync.RWMutex
	oomNotifier *exec.Cmd

	currentBandwidthLimits *warden.BandwidthLimits
	bandwidthMutex         sync.RWMutex

	currentDiskLimits *warden.DiskLimits
	diskMutex         sync.RWMutex

	currentMemoryLimits *warden.MemoryLimits
	memoryMutex         sync.RWMutex

	currentCPULimits *warden.CPULimits
	cpuMutex         sync.RWMutex

	netIns      []NetInSpec
	netInsMutex sync.RWMutex

	netOuts      []NetOutSpec
	netOutsMutex sync.RWMutex
}

type NetInSpec struct {
	HostPort      uint32
	ContainerPort uint32
}

type NetOutSpec struct {
	Network string
	Port    uint32
}

type PortPool interface {
	Acquire() (uint32, error)
	Remove(uint32) error
	Release(uint32)
}

type State string

const (
	StateBorn    = State("born")
	StateActive  = State("active")
	StateStopped = State("stopped")
)

func NewLinuxContainer(
	id, handle, path string,
	properties warden.Properties,
	graceTime time.Duration,
	resources *Resources,
	portPool PortPool,
	runner command_runner.CommandRunner,
	cgroupsManager cgroups_manager.CgroupsManager,
	quotaManager quota_manager.QuotaManager,
	bandwidthManager bandwidth_manager.BandwidthManager,
	processTracker process_tracker.ProcessTracker,
) *LinuxContainer {
	return &LinuxContainer{
		id:     id,
		handle: handle,
		path:   path,

		properties: properties,

		graceTime: graceTime,

		state:  StateBorn,
		events: []string{},

		resources: resources,

		portPool: portPool,

		runner: runner,

		cgroupsManager:   cgroupsManager,
		quotaManager:     quotaManager,
		bandwidthManager: bandwidthManager,

		processTracker: processTracker, //process_tracker.New(path, runner),
	}
}

func (c *LinuxContainer) ID() string {
	return c.id
}

func (c *LinuxContainer) Handle() string {
	return c.handle
}

func (c *LinuxContainer) GraceTime() time.Duration {
	return c.graceTime
}

func (c *LinuxContainer) Properties() warden.Properties {
	return c.properties
}

func (c *LinuxContainer) State() State {
	c.stateMutex.RLock()
	defer c.stateMutex.RUnlock()

	return c.state
}

func (c *LinuxContainer) Events() []string {
	c.eventsMutex.RLock()
	defer c.eventsMutex.RUnlock()

	events := make([]string, len(c.events))

	copy(events, c.events)

	return events
}

func (c *LinuxContainer) Resources() *Resources {
	return c.resources
}

func (c *LinuxContainer) Snapshot(out io.Writer) error {
	c.bandwidthMutex.RLock()
	defer c.bandwidthMutex.RUnlock()

	c.cpuMutex.RLock()
	defer c.cpuMutex.RUnlock()

	c.diskMutex.RLock()
	defer c.diskMutex.RUnlock()

	c.memoryMutex.RLock()
	defer c.memoryMutex.RUnlock()

	c.netInsMutex.RLock()
	defer c.netInsMutex.RUnlock()

	c.netOutsMutex.RLock()
	defer c.netOutsMutex.RUnlock()

	processSnapshots := []ProcessSnapshot{}

	for _, id := range c.processTracker.ActiveProcessIDs() {
		processSnapshots = append(
			processSnapshots,
			ProcessSnapshot{ID: id},
		)
	}

	return json.NewEncoder(out).Encode(
		ContainerSnapshot{
			ID:     c.id,
			Handle: c.handle,

			GraceTime: c.graceTime,

			State:  string(c.State()),
			Events: c.Events(),

			Limits: LimitsSnapshot{
				Bandwidth: c.currentBandwidthLimits,
				CPU:       c.currentCPULimits,
				Disk:      c.currentDiskLimits,
				Memory:    c.currentMemoryLimits,
			},

			Resources: ResourcesSnapshot{
				UID:     c.resources.UID,
				Network: c.resources.Network,
				Ports:   c.resources.Ports,
			},

			NetIns:  c.netIns,
			NetOuts: c.netOuts,

			Processes: processSnapshots,

			Properties: c.Properties(),
		},
	)
}

func (c *LinuxContainer) Restore(snapshot ContainerSnapshot) error {
	c.setState(State(snapshot.State))

	for _, ev := range snapshot.Events {
		c.registerEvent(ev)
	}

	if snapshot.Limits.Memory != nil {
		err := c.LimitMemory(*snapshot.Limits.Memory)
		if err != nil {
			return err
		}
	}

	for _, process := range snapshot.Processes {
		c.processTracker.Restore(process.ID)
	}

	net := &exec.Cmd{
		Path: path.Join(c.path, "net.sh"),
		Args: []string{"setup"},
	}

	err := c.runner.Run(net)
	if err != nil {
		return err
	}

	for _, in := range snapshot.NetIns {
		_, _, err = c.NetIn(in.HostPort, in.ContainerPort)
		if err != nil {
			return err
		}
	}

	for _, out := range snapshot.NetOuts {
		err = c.NetOut(out.Network, out.Port)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *LinuxContainer) Start() error {
	log.Println(c.id, "starting")

	start := &exec.Cmd{
		Path: path.Join(c.path, "start.sh"),
		Env: []string{
			"id=" + c.id,
			"container_iface_mtu=1500",
			"PATH=" + os.Getenv("PATH"),
		},
	}

	err := c.runner.Run(start)
	if err != nil {
		return err
	}

	c.setState(StateActive)

	return nil
}

func (c *LinuxContainer) Stop(kill bool) error {
	log.Println(c.id, "stopping")

	stop := &exec.Cmd{
		Path: path.Join(c.path, "stop.sh"),
	}

	if kill {
		stop.Args = append(stop.Args, "-w", "0")
	}

	err := c.runner.Run(stop)
	if err != nil {
		return err
	}

	c.stopOomNotifier()

	c.setState(StateStopped)

	return nil
}

func (c *LinuxContainer) Cleanup() {
	c.stopOomNotifier()

	c.processTracker.UnlinkAll()
}

func (c *LinuxContainer) Info() (warden.ContainerInfo, error) {
	log.Println(c.id, "info")

	memoryStat, err := c.cgroupsManager.Get("memory", "memory.stat")
	if err != nil {
		return warden.ContainerInfo{}, err
	}

	cpuUsage, err := c.cgroupsManager.Get("cpuacct", "cpuacct.usage")
	if err != nil {
		return warden.ContainerInfo{}, err
	}

	cpuStat, err := c.cgroupsManager.Get("cpuacct", "cpuacct.stat")
	if err != nil {
		return warden.ContainerInfo{}, err
	}

	diskStat, err := c.quotaManager.GetUsage(c.resources.UID)
	if err != nil {
		return warden.ContainerInfo{}, err
	}

	bandwidthStat, err := c.bandwidthManager.GetLimits()
	if err != nil {
		return warden.ContainerInfo{}, err
	}

	mappedPorts := []warden.PortMapping{}

	c.netInsMutex.RLock()

	for _, spec := range c.netIns {
		mappedPorts = append(mappedPorts, warden.PortMapping{
			HostPort:      spec.HostPort,
			ContainerPort: spec.ContainerPort,
		})
	}

	c.netInsMutex.RUnlock()

	return warden.ContainerInfo{
		State:         string(c.State()),
		Events:        c.Events(),
		Properties:    c.Properties(),
		HostIP:        c.resources.Network.HostIP().String(),
		ContainerIP:   c.resources.Network.ContainerIP().String(),
		ContainerPath: c.path,
		ProcessIDs:    c.processTracker.ActiveProcessIDs(),
		MemoryStat:    parseMemoryStat(memoryStat),
		CPUStat:       parseCPUStat(cpuUsage, cpuStat),
		DiskStat:      diskStat,
		BandwidthStat: bandwidthStat,
		MappedPorts:   mappedPorts,
	}, nil
}

type closeTracker struct {
	io.WriteCloser

	callback func() error
}

func (tracker closeTracker) Close() error {
	err := tracker.WriteCloser.Close()
	if err != nil {
		return err
	}

	return tracker.callback()
}

func (c *LinuxContainer) StreamIn(dstPath string) (io.WriteCloser, error) {
	log.Println(c.id, "writing data to:", dstPath)

	wshPath := path.Join(c.path, "bin", "wsh")
	sockPath := path.Join(c.path, "run", "wshd.sock")

	tar := &exec.Cmd{
		Path: wshPath,
		Args: []string{
			"--socket", sockPath,
			"--user", "vcap",
			"bash", "-c",
			fmt.Sprintf("mkdir -p %s && tar xf - -C %s", dstPath, dstPath),
		},
	}

	tarWrite, err := tar.StdinPipe()
	if err != nil {
		return nil, err
	}

	err = c.runner.Background(tar)
	if err != nil {
		return nil, err
	}

	errorChan := make(chan error, 1)
	go func() {
		errorChan <- c.runner.Wait(tar)
	}()

	return closeTracker{
		WriteCloser: tarWrite,
		callback: func() error {
			return <-errorChan
		},
	}, nil
}

func (c *LinuxContainer) StreamOut(srcPath string) (io.Reader, error) {
	log.Println(c.id, "reading data from:", srcPath)

	wshPath := path.Join(c.path, "bin", "wsh")
	sockPath := path.Join(c.path, "run", "wshd.sock")

	workingDir := filepath.Dir(srcPath)
	compressArg := filepath.Base(srcPath)
	if strings.HasSuffix(srcPath, "/") {
		workingDir = srcPath
		compressArg = "."
	}

	tarRead, tarWrite := io.Pipe()

	tar := &exec.Cmd{
		Path: wshPath,
		Args: []string{
			"--socket", sockPath,
			"--user", "vcap",
			"tar", "cf", "-", "-C", workingDir, compressArg,
		},
		Stdout: tarWrite,
	}

	err := c.runner.Background(tar)
	if err != nil {
		return nil, err
	}

	go func() {
		c.runner.Wait(tar)
		tarWrite.Close()
	}()

	return tarRead, nil
}

func (c *LinuxContainer) LimitBandwidth(limits warden.BandwidthLimits) error {
	log.Println(
		c.id,
		"limiting bandwidth to",
		limits.RateInBytesPerSecond,
		"bytes per second; burst",
		limits.BurstRateInBytesPerSecond,
	)

	err := c.bandwidthManager.SetLimits(limits)
	if err != nil {
		return err
	}

	c.bandwidthMutex.Lock()
	defer c.bandwidthMutex.Unlock()

	c.currentBandwidthLimits = &limits

	return nil
}

func (c *LinuxContainer) CurrentBandwidthLimits() (warden.BandwidthLimits, error) {
	c.bandwidthMutex.RLock()
	defer c.bandwidthMutex.RUnlock()

	if c.currentBandwidthLimits == nil {
		return warden.BandwidthLimits{}, nil
	}

	return *c.currentBandwidthLimits, nil
}

func (c *LinuxContainer) LimitDisk(limits warden.DiskLimits) error {
	log.Println(c.id, "limiting disk", limits)

	err := c.quotaManager.SetLimits(c.resources.UID, limits)
	if err != nil {
		return err
	}

	c.diskMutex.Lock()
	defer c.diskMutex.Unlock()

	c.currentDiskLimits = &limits

	return nil
}

func (c *LinuxContainer) CurrentDiskLimits() (warden.DiskLimits, error) {
	return c.quotaManager.GetLimits(c.resources.UID)
}

func (c *LinuxContainer) LimitMemory(limits warden.MemoryLimits) error {
	log.Println(c.id, "limiting memory to", limits.LimitInBytes, "bytes")

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

	c.currentMemoryLimits = &limits

	return nil
}

func (c *LinuxContainer) CurrentMemoryLimits() (warden.MemoryLimits, error) {
	limitInBytes, err := c.cgroupsManager.Get("memory", "memory.limit_in_bytes")
	if err != nil {
		return warden.MemoryLimits{}, err
	}

	numericLimit, err := strconv.ParseUint(limitInBytes, 10, 0)
	if err != nil {
		return warden.MemoryLimits{}, err
	}

	return warden.MemoryLimits{uint64(numericLimit)}, nil
}

func (c *LinuxContainer) LimitCPU(limits warden.CPULimits) error {
	log.Println(c.id, "limiting CPU to", limits.LimitInShares, "shares")

	limit := fmt.Sprintf("%d", limits.LimitInShares)

	err := c.cgroupsManager.Set("cpu", "cpu.shares", limit)
	if err != nil {
		return err
	}

	c.cpuMutex.Lock()
	defer c.cpuMutex.Unlock()

	c.currentCPULimits = &limits

	return nil
}

func (c *LinuxContainer) CurrentCPULimits() (warden.CPULimits, error) {
	actualLimitInShares, err := c.cgroupsManager.Get("cpu", "cpu.shares")
	if err != nil {
		return warden.CPULimits{}, err
	}

	numericLimit, err := strconv.ParseUint(actualLimitInShares, 10, 0)
	if err != nil {
		return warden.CPULimits{}, err
	}

	return warden.CPULimits{uint64(numericLimit)}, nil
}

func (c *LinuxContainer) Run(spec warden.ProcessSpec) (uint32, <-chan warden.ProcessStream, error) {
	script := ""

	for _, env := range spec.EnvironmentVariables {
		script += exportCommand(env)
	}

	script += spec.Script

	log.Println(c.id, "running process:", script)

	wshPath := path.Join(c.path, "bin", "wsh")
	sockPath := path.Join(c.path, "run", "wshd.sock")

	user := "vcap"
	if spec.Privileged {
		user = "root"
	}

	wsh := &exec.Cmd{
		Path:  wshPath,
		Args:  []string{"--socket", sockPath, "--user", user, "/bin/bash"},
		Stdin: bytes.NewBufferString(script),
	}

	setRLimitsEnv(wsh, spec.Limits)

	return c.processTracker.Run(wsh)
}

func (c *LinuxContainer) Attach(processID uint32) (<-chan warden.ProcessStream, error) {
	log.Println(c.id, "attaching to process", processID)
	return c.processTracker.Attach(processID)
}

func (c *LinuxContainer) NetIn(hostPort uint32, containerPort uint32) (uint32, uint32, error) {
	if hostPort == 0 {
		randomPort, err := c.portPool.Acquire()
		if err != nil {
			return 0, 0, err
		}

		c.resources.AddPort(randomPort)

		hostPort = randomPort
	}

	if containerPort == 0 {
		containerPort = hostPort
	}

	log.Println(
		c.id,
		"mapping host port",
		hostPort,
		"to container port",
		containerPort,
	)

	net := &exec.Cmd{
		Path: path.Join(c.path, "net.sh"),
		Args: []string{"in"},
		Env: []string{
			fmt.Sprintf("HOST_PORT=%d", hostPort),
			fmt.Sprintf("CONTAINER_PORT=%d", containerPort),
			"PATH=" + os.Getenv("PATH"),
		},
	}

	err := c.runner.Run(net)
	if err != nil {
		return 0, 0, err
	}

	c.netInsMutex.Lock()
	defer c.netInsMutex.Unlock()

	c.netIns = append(c.netIns, NetInSpec{hostPort, containerPort})

	return hostPort, containerPort, nil
}

func (c *LinuxContainer) NetOut(network string, port uint32) error {
	net := &exec.Cmd{
		Path: path.Join(c.path, "net.sh"),
		Args: []string{"out"},
	}

	if port != 0 {
		log.Println(
			c.id,
			"permitting traffic to",
			network,
			"with port",
			port,
		)

		net.Env = []string{
			"NETWORK=" + network,
			fmt.Sprintf("PORT=%d", port),
			"PATH=" + os.Getenv("PATH"),
		}
	} else {
		if network == "" {
			return fmt.Errorf("network and/or port must be provided")
		}

		log.Println(c.id, "permitting traffic to", network)

		net.Env = []string{
			"NETWORK=" + network,
			"PORT=",
			"PATH=" + os.Getenv("PATH"),
		}
	}

	err := c.runner.Run(net)
	if err != nil {
		return err
	}

	c.netOutsMutex.Lock()
	defer c.netOutsMutex.Unlock()

	c.netOuts = append(c.netOuts, NetOutSpec{network, port})

	return nil
}

func (c *LinuxContainer) setState(state State) {
	c.stateMutex.Lock()
	defer c.stateMutex.Unlock()

	c.state = state
}

func (c *LinuxContainer) registerEvent(event string) {
	c.eventsMutex.Lock()
	defer c.eventsMutex.Unlock()

	c.events = append(c.events, event)
}

func (c *LinuxContainer) startOomNotifier() error {
	c.oomMutex.Lock()
	defer c.oomMutex.Unlock()

	if c.oomNotifier != nil {
		return nil
	}

	oomPath := path.Join(c.path, "bin", "oom")

	c.oomNotifier = &exec.Cmd{
		Path: oomPath,
		Args: []string{c.cgroupsManager.SubsystemPath("memory")},
	}

	err := c.runner.Start(c.oomNotifier)
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
		log.Println(c.id, "out of memory")
		c.registerEvent("out of memory")
		c.Stop(false)
	} else {
		log.Println(c.id, "oom failed:", err)
	}

	// TODO: handle case where oom notifier itself failed? kill container?
}

func parseMemoryStat(contents string) (stat warden.ContainerMemoryStat) {
	scanner := bufio.NewScanner(strings.NewReader(contents))

	scanner.Split(bufio.ScanWords)

	for scanner.Scan() {
		field := scanner.Text()

		if !scanner.Scan() {
			break
		}

		value, err := strconv.ParseUint(scanner.Text(), 10, 0)
		if err != nil {
			continue
		}

		switch field {
		case "cache":
			stat.Cache = value
		case "rss":
			stat.Rss = value
		case "mapped_file":
			stat.MappedFile = value
		case "pgpgin":
			stat.Pgpgin = value
		case "pgpgout":
			stat.Pgpgout = value
		case "swap":
			stat.Swap = value
		case "pgfault":
			stat.Pgfault = value
		case "pgmajfault":
			stat.Pgmajfault = value
		case "inactive_anon":
			stat.InactiveAnon = value
		case "active_anon":
			stat.ActiveAnon = value
		case "inactive_file":
			stat.InactiveFile = value
		case "active_file":
			stat.ActiveFile = value
		case "unevictable":
			stat.Unevictable = value
		case "hierarchical_memory_limit":
			stat.HierarchicalMemoryLimit = value
		case "hierarchical_memsw_limit":
			stat.HierarchicalMemswLimit = value
		case "total_cache":
			stat.TotalCache = value
		case "total_rss":
			stat.TotalRss = value
		case "total_mapped_file":
			stat.TotalMappedFile = value
		case "total_pgpgin":
			stat.TotalPgpgin = value
		case "total_pgpgout":
			stat.TotalPgpgout = value
		case "total_swap":
			stat.TotalSwap = value
		case "total_pgfault":
			stat.TotalPgfault = value
		case "total_pgmajfault":
			stat.TotalPgmajfault = value
		case "total_inactive_anon":
			stat.TotalInactiveAnon = value
		case "total_active_anon":
			stat.TotalActiveAnon = value
		case "total_inactive_file":
			stat.TotalInactiveFile = value
		case "total_active_file":
			stat.TotalActiveFile = value
		case "total_unevictable":
			stat.TotalUnevictable = value
		}
	}

	return
}

func parseCPUStat(usage, statContents string) (stat warden.ContainerCPUStat) {
	cpuUsage, err := strconv.ParseUint(strings.Trim(usage, "\n"), 10, 0)
	if err != nil {
		return
	}

	stat.Usage = cpuUsage

	scanner := bufio.NewScanner(strings.NewReader(statContents))

	scanner.Split(bufio.ScanWords)

	for scanner.Scan() {
		field := scanner.Text()

		if !scanner.Scan() {
			break
		}

		value, err := strconv.ParseUint(scanner.Text(), 10, 0)
		if err != nil {
			continue
		}

		switch field {
		case "user":
			stat.User = value
		case "system":
			stat.System = value
		}
	}

	return
}

func setRLimitsEnv(cmd *exec.Cmd, rlimits warden.ResourceLimits) {
	if rlimits.As != nil {
		cmd.Env = append(cmd.Env, fmt.Sprintf("RLIMIT_AS=%d", *rlimits.As))
	}

	if rlimits.Core != nil {
		cmd.Env = append(cmd.Env, fmt.Sprintf("RLIMIT_CORE=%d", *rlimits.Core))
	}

	if rlimits.Cpu != nil {
		cmd.Env = append(cmd.Env, fmt.Sprintf("RLIMIT_CPU=%d", *rlimits.Cpu))
	}

	if rlimits.Data != nil {
		cmd.Env = append(cmd.Env, fmt.Sprintf("RLIMIT_DATA=%d", *rlimits.Data))
	}

	if rlimits.Fsize != nil {
		cmd.Env = append(cmd.Env, fmt.Sprintf("RLIMIT_FSIZE=%d", *rlimits.Fsize))
	}

	if rlimits.Locks != nil {
		cmd.Env = append(cmd.Env, fmt.Sprintf("RLIMIT_LOCKS=%d", *rlimits.Locks))
	}

	if rlimits.Memlock != nil {
		cmd.Env = append(cmd.Env, fmt.Sprintf("RLIMIT_MEMLOCK=%d", *rlimits.Memlock))
	}

	if rlimits.Msgqueue != nil {
		cmd.Env = append(cmd.Env, fmt.Sprintf("RLIMIT_MSGQUEUE=%d", *rlimits.Msgqueue))
	}

	if rlimits.Nice != nil {
		cmd.Env = append(cmd.Env, fmt.Sprintf("RLIMIT_NICE=%d", *rlimits.Nice))
	}

	if rlimits.Nofile != nil {
		cmd.Env = append(cmd.Env, fmt.Sprintf("RLIMIT_NOFILE=%d", *rlimits.Nofile))
	}

	if rlimits.Nproc != nil {
		cmd.Env = append(cmd.Env, fmt.Sprintf("RLIMIT_NPROC=%d", *rlimits.Nproc))
	}

	if rlimits.Rss != nil {
		cmd.Env = append(cmd.Env, fmt.Sprintf("RLIMIT_RSS=%d", *rlimits.Rss))
	}

	if rlimits.Rtprio != nil {
		cmd.Env = append(cmd.Env, fmt.Sprintf("RLIMIT_RTPRIO=%d", *rlimits.Rtprio))
	}

	if rlimits.Sigpending != nil {
		cmd.Env = append(cmd.Env, fmt.Sprintf("RLIMIT_SIGPENDING=%d", *rlimits.Sigpending))
	}

	if rlimits.Stack != nil {
		cmd.Env = append(cmd.Env, fmt.Sprintf("RLIMIT_STACK=%d", *rlimits.Stack))
	}
}

func exportCommand(env warden.EnvironmentVariable) string {
	return fmt.Sprintf("export %s=\"%s\"\n", env.Key, escapeQuotes(env.Value))
}

func escapeQuotes(value string) string {
	return strings.Replace(value, `"`, `\"`, -1)
}
