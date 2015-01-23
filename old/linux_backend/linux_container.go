package linux_backend

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry-incubator/garden-linux/fences/netfence/network"
	"github.com/cloudfoundry-incubator/garden-linux/old/linux_backend/bandwidth_manager"
	"github.com/cloudfoundry-incubator/garden-linux/old/linux_backend/cgroups_manager"
	"github.com/cloudfoundry-incubator/garden-linux/old/linux_backend/process_tracker"
	"github.com/cloudfoundry-incubator/garden-linux/old/linux_backend/quota_manager"
	"github.com/cloudfoundry-incubator/garden-linux/old/logging"
	"github.com/cloudfoundry-incubator/garden-linux/process"
	"github.com/cloudfoundry/gunk/command_runner"
	"github.com/pivotal-golang/lager"
)

type UndefinedPropertyError struct {
	Key string
}

func (err UndefinedPropertyError) Error() string {
	return fmt.Sprintf("property does not exist: %s", err.Key)
}

type LinuxContainer struct {
	logger lager.Logger

	id     string
	handle string
	path   string

	properties      garden.Properties
	propertiesMutex sync.RWMutex

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

	filter network.Filter

	oomMutex    sync.RWMutex
	oomNotifier *exec.Cmd

	currentBandwidthLimits *garden.BandwidthLimits
	bandwidthMutex         sync.RWMutex

	currentDiskLimits *garden.DiskLimits
	diskMutex         sync.RWMutex

	currentMemoryLimits *garden.MemoryLimits
	memoryMutex         sync.RWMutex

	currentCPULimits *garden.CPULimits
	cpuMutex         sync.RWMutex

	netIns      []NetInSpec
	netInsMutex sync.RWMutex

	netOuts      []NetOutSpec
	netOutsMutex sync.RWMutex

	mtu uint32

	env process.Env

	processIDPool *ProcessIDPool
}

type ProcessIDPool struct {
	currentProcessID uint32
	mu               sync.Mutex
}

func (p *ProcessIDPool) Next() uint32 {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.currentProcessID = p.currentProcessID + 1
	return p.currentProcessID
}

func (p *ProcessIDPool) Restore(id uint32) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if id >= p.currentProcessID {
		p.currentProcessID = id
	}
}

type NetInSpec struct {
	HostPort      uint32
	ContainerPort uint32
}

// TODO: extend this for security groups https://www.pivotaltracker.com/story/show/82554270
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
	logger lager.Logger,
	id, handle, path string,
	properties garden.Properties,
	graceTime time.Duration,
	resources *Resources,
	portPool PortPool,
	runner command_runner.CommandRunner,
	cgroupsManager cgroups_manager.CgroupsManager,
	quotaManager quota_manager.QuotaManager,
	bandwidthManager bandwidth_manager.BandwidthManager,
	processTracker process_tracker.ProcessTracker,
	env process.Env,
	filter network.Filter,
) *LinuxContainer {
	return &LinuxContainer{
		logger: logger,

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

		processTracker: processTracker,

		filter: filter,

		env:           env,
		processIDPool: &ProcessIDPool{},
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

func (c *LinuxContainer) Properties() garden.Properties {
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
	cLog := c.logger.Session("snapshot")

	cLog.Debug("saving")

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

	for _, p := range c.processTracker.ActiveProcesses() {
		processSnapshots = append(
			processSnapshots,
			ProcessSnapshot{
				ID: p.ID(),
			},
		)
	}

	snapshot := ContainerSnapshot{
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
			UserUID: c.resources.UserUID,
			RootUID: c.resources.RootUID,
			Ports:   c.resources.Ports,
		},

		NetIns:  c.netIns,
		NetOuts: c.netOuts,

		Processes: processSnapshots,

		Properties: c.Properties(),

		EnvVars: c.env.Array(),
	}

	var err error
	m, err := c.resources.Network.MarshalJSON()
	if err != nil {
		cLog.Error("failed-to-save", err, lager.Data{
			"snapshot": snapshot,
			"network":  c.resources.Network,
		})
		return err
	}

	var rm json.RawMessage = m
	snapshot.Resources.Network = &rm

	err = json.NewEncoder(out).Encode(snapshot)
	if err != nil {
		cLog.Error("failed-to-save", err, lager.Data{
			"snapshot": snapshot,
		})
		return err
	}

	cLog.Info("saved", lager.Data{
		"snapshot": snapshot,
	})

	return nil
}

func (c *LinuxContainer) Restore(snapshot ContainerSnapshot) error {
	cLog := c.logger.Session("restore")

	cLog.Debug("restoring")

	cRunner := logging.Runner{
		CommandRunner: c.runner,
		Logger:        cLog,
	}

	c.setState(State(snapshot.State))

	snapshotEnv, err := process.NewEnv(snapshot.EnvVars)
	if err != nil {
		cLog.Error("restoring-env", err, lager.Data{
			"env": snapshot.EnvVars,
		})
		return err
	}
	c.env = snapshotEnv

	for _, ev := range snapshot.Events {
		c.registerEvent(ev)
	}

	if snapshot.Limits.Memory != nil {
		err := c.LimitMemory(*snapshot.Limits.Memory)
		if err != nil {
			cLog.Error("failed-to-limit-memory", err)
			return err
		}
	}

	for _, process := range snapshot.Processes {
		cLog.Info("restoring-process", lager.Data{
			"process": process,
		})

		c.processIDPool.Restore(process.ID)

		pidfile := path.Join(c.path, "processes", fmt.Sprintf("%d.pid", process.ID))

		signaller := &NamespacedSignaller{
			Runner:        c.runner,
			ContainerPath: c.path,
			PidFilePath:   pidfile,
		}

		c.processTracker.Restore(process.ID, signaller)
	}

	net := exec.Command(path.Join(c.path, "net.sh"), "setup")

	err = cRunner.Run(net)
	if err != nil {
		cLog.Error("failed-to-reenforce-network-rules", err)
		return err
	}

	for _, in := range snapshot.NetIns {
		_, _, err = c.NetIn(in.HostPort, in.ContainerPort)
		if err != nil {
			cLog.Error("failed-to-reenforce-port-mapping", err)
			return err
		}
	}

	for _, out := range snapshot.NetOuts {
		err = c.NetOut(out.Network, out.Port, "", garden.ProtocolTCP, -1, -1, false)
		if err != nil {
			cLog.Error("failed-to-reenforce-allowed-traffic", err)
			return err
		}
	}

	cLog.Info("restored")

	return nil
}

func (c *LinuxContainer) Start() error {
	cLog := c.logger.Session("start")

	cLog.Debug("starting")

	start := exec.Command(path.Join(c.path, "start.sh"))
	start.Env = []string{
		"id=" + c.id,
		"PATH=" + os.Getenv("PATH"),
	}

	cRunner := logging.Runner{
		CommandRunner: c.runner,
		Logger:        cLog,
	}

	err := cRunner.Run(start)
	if err != nil {
		cLog.Error("failed-to-start", err)
		return fmt.Errorf("container: start: %v", err)
	}

	c.setState(StateActive)

	cLog.Info("started")

	return nil
}

func (c *LinuxContainer) Cleanup() {
	cLog := c.logger.Session("cleanup")

	cLog.Debug("stopping-oom-notifier")
	c.stopOomNotifier()

	cLog.Info("done")
}

func (c *LinuxContainer) Stop(kill bool) error {
	stop := exec.Command(path.Join(c.path, "stop.sh"))

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

func (c *LinuxContainer) GetProperty(key string) (string, error) {
	c.propertiesMutex.RLock()
	defer c.propertiesMutex.RUnlock()

	value, found := c.properties[key]
	if !found {
		return "", UndefinedPropertyError{key}
	}

	return value, nil
}

func (c *LinuxContainer) SetProperty(key string, value string) error {
	c.propertiesMutex.Lock()
	defer c.propertiesMutex.Unlock()

	c.properties[key] = value

	return nil
}

func (c *LinuxContainer) RemoveProperty(key string) error {
	c.propertiesMutex.Lock()
	defer c.propertiesMutex.Unlock()

	_, found := c.properties[key]
	if !found {
		return UndefinedPropertyError{key}
	}

	delete(c.properties, key)

	return nil
}

func (c *LinuxContainer) Info() (garden.ContainerInfo, error) {
	cLog := c.logger.Session("info")

	memoryStat, err := c.cgroupsManager.Get("memory", "memory.stat")
	if err != nil {
		return garden.ContainerInfo{}, err
	}

	cpuUsage, err := c.cgroupsManager.Get("cpuacct", "cpuacct.usage")
	if err != nil {
		return garden.ContainerInfo{}, err
	}

	cpuStat, err := c.cgroupsManager.Get("cpuacct", "cpuacct.stat")
	if err != nil {
		return garden.ContainerInfo{}, err
	}

	diskStat, err := c.quotaManager.GetUsage(cLog, c.resources.UserUID)
	if err != nil {
		return garden.ContainerInfo{}, err
	}

	bandwidthStat, err := c.bandwidthManager.GetLimits(cLog)
	if err != nil {
		return garden.ContainerInfo{}, err
	}

	mappedPorts := []garden.PortMapping{}

	c.netInsMutex.RLock()

	for _, spec := range c.netIns {
		mappedPorts = append(mappedPorts, garden.PortMapping{
			HostPort:      spec.HostPort,
			ContainerPort: spec.ContainerPort,
		})
	}

	c.netInsMutex.RUnlock()

	processIDs := []uint32{}
	for _, process := range c.processTracker.ActiveProcesses() {
		processIDs = append(processIDs, process.ID())
	}

	info := garden.ContainerInfo{
		State:         string(c.State()),
		Events:        c.Events(),
		Properties:    c.Properties(),
		ContainerPath: c.path,
		ProcessIDs:    processIDs,
		MemoryStat:    parseMemoryStat(memoryStat),
		CPUStat:       parseCPUStat(cpuUsage, cpuStat),
		DiskStat:      diskStat,
		BandwidthStat: bandwidthStat,
		MappedPorts:   mappedPorts,
	}

	c.Resources().Network.Info(&info)
	return info, nil
}

func (c *LinuxContainer) StreamIn(dstPath string, tarStream io.Reader) error {
	nsTarPath := path.Join(c.path, "bin", "nstar")
	pidPath := path.Join(c.path, "run", "wshd.pid")

	pidFile, err := os.Open(pidPath)
	if err != nil {
		return err
	}

	var pid int
	_, err = fmt.Fscanf(pidFile, "%d", &pid)
	if err != nil {
		return err
	}

	tar := exec.Command(
		nsTarPath,
		strconv.Itoa(pid),
		"vcap",
		dstPath,
	)

	tar.Stdin = tarStream

	cLog := c.logger.Session("stream-in")

	cRunner := logging.Runner{
		CommandRunner: c.runner,
		Logger:        cLog,
	}

	return cRunner.Run(tar)
}

func (c *LinuxContainer) StreamOut(srcPath string) (io.ReadCloser, error) {
	workingDir := filepath.Dir(srcPath)
	compressArg := filepath.Base(srcPath)
	if strings.HasSuffix(srcPath, "/") {
		workingDir = srcPath
		compressArg = "."
	}

	nsTarPath := path.Join(c.path, "bin", "nstar")
	pidPath := path.Join(c.path, "run", "wshd.pid")

	pidFile, err := os.Open(pidPath)
	if err != nil {
		return nil, err
	}

	var pid int
	_, err = fmt.Fscanf(pidFile, "%d", &pid)
	if err != nil {
		return nil, err
	}

	tar := exec.Command(
		nsTarPath,
		strconv.Itoa(pid),
		"vcap",
		workingDir,
		compressArg,
	)

	tarRead, tarWrite, err := os.Pipe()
	if err != nil {
		return nil, err
	}

	tar.Stdout = tarWrite

	err = c.runner.Background(tar)
	if err != nil {
		return nil, err
	}

	// close our end of the tar pipe
	tarWrite.Close()

	go c.runner.Wait(tar)

	return tarRead, nil
}

func (c *LinuxContainer) LimitBandwidth(limits garden.BandwidthLimits) error {
	cLog := c.logger.Session("limit-bandwidth")

	err := c.bandwidthManager.SetLimits(cLog, limits)
	if err != nil {
		return err
	}

	c.bandwidthMutex.Lock()
	defer c.bandwidthMutex.Unlock()

	c.currentBandwidthLimits = &limits

	return nil
}

func (c *LinuxContainer) CurrentBandwidthLimits() (garden.BandwidthLimits, error) {
	c.bandwidthMutex.RLock()
	defer c.bandwidthMutex.RUnlock()

	if c.currentBandwidthLimits == nil {
		return garden.BandwidthLimits{}, nil
	}

	return *c.currentBandwidthLimits, nil
}

func (c *LinuxContainer) LimitDisk(limits garden.DiskLimits) error {
	cLog := c.logger.Session("limit-disk")

	err := c.quotaManager.SetLimits(cLog, c.resources.UserUID, limits)
	if err != nil {
		return err
	}

	c.diskMutex.Lock()
	defer c.diskMutex.Unlock()

	c.currentDiskLimits = &limits

	return nil
}

func (c *LinuxContainer) CurrentDiskLimits() (garden.DiskLimits, error) {
	cLog := c.logger.Session("current-disk-limits")
	return c.quotaManager.GetLimits(cLog, c.resources.UserUID)
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

	c.currentMemoryLimits = &limits

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

	c.currentCPULimits = &limits

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

func (c *LinuxContainer) Run(spec garden.ProcessSpec, processIO garden.ProcessIO) (garden.Process, error) {
	wshPath := path.Join(c.path, "bin", "wsh")
	sockPath := path.Join(c.path, "run", "wshd.sock")

	user := "vcap"
	if spec.Privileged {
		user = "root"
	}

	if spec.User != "" {
		user = spec.User
	}

	args := []string{"--socket", sockPath, "--user", user}

	specEnv, err := process.NewEnv(spec.Env)
	if err != nil {
		return nil, err
	}

	for _, envVar := range c.env.Merge(specEnv).Array() {
		args = append(args, "--env", envVar)
	}

	if spec.Dir != "" {
		args = append(args, "--dir", spec.Dir)
	}

	processID := c.processIDPool.Next()

	pidfile := path.Join(c.path, "processes", fmt.Sprintf("%d.pid", processID))
	args = append(args, "--pidfile", pidfile)

	signaller := &NamespacedSignaller{
		Runner:        c.runner,
		ContainerPath: c.path,
		PidFilePath:   pidfile,
	}

	args = append(args, spec.Path)

	wsh := exec.Command(wshPath, append(args, spec.Args...)...)

	setRLimitsEnv(wsh, spec.Limits)

	return c.processTracker.Run(processID, wsh, processIO, spec.TTY, signaller)
}

func (c *LinuxContainer) Attach(processID uint32, processIO garden.ProcessIO) (garden.Process, error) {
	return c.processTracker.Attach(processID, processIO)
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

	net := exec.Command(path.Join(c.path, "net.sh"), "in")
	net.Env = []string{
		fmt.Sprintf("HOST_PORT=%d", hostPort),
		fmt.Sprintf("CONTAINER_PORT=%d", containerPort),
		"PATH=" + os.Getenv("PATH"),
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

func (c *LinuxContainer) NetOut(network string, port uint32, portRange string, protocol garden.Protocol, icmpType int32, icmpCode int32, log bool) error {
	err := c.filter.NetOut(network, port, portRange, protocol, icmpType, icmpCode, log)
	if err != nil {
		return err
	}

	c.netOutsMutex.Lock()
	defer c.netOutsMutex.Unlock()

	c.netOuts = append(c.netOuts, NetOutSpec{network, port})

	return nil
}

func (c *LinuxContainer) CurrentEnvVars() []string {
	return c.env.Array()
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

	c.oomNotifier = exec.Command(oomPath, c.cgroupsManager.SubsystemPath("memory"))

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
		c.registerEvent("out of memory")
		c.Stop(false)
	}

	// TODO: handle case where oom notifier itself failed? kill container?
}

func parseMemoryStat(contents string) (stat garden.ContainerMemoryStat) {
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

func parseCPUStat(usage, statContents string) (stat garden.ContainerCPUStat) {
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

func setRLimitsEnv(cmd *exec.Cmd, rlimits garden.ResourceLimits) {
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
