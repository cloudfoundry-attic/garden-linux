package linux_container

import (
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
	"github.com/cloudfoundry-incubator/garden-linux/linux_backend"
	"github.com/cloudfoundry-incubator/garden-linux/logging"
	"github.com/cloudfoundry-incubator/garden-linux/network"
	"github.com/cloudfoundry-incubator/garden-linux/network/subnets"
	"github.com/cloudfoundry-incubator/garden-linux/process_tracker"
	"github.com/cloudfoundry/gunk/command_runner"
	"github.com/pivotal-golang/lager"
)

type UndefinedPropertyError struct {
	Key string
}

func (err UndefinedPropertyError) Error() string {
	return fmt.Sprintf("property does not exist: %s", err.Key)
}

//go:generate counterfeiter -o fake_quota_manager/fake_quota_manager.go . QuotaManager
type QuotaManager interface {
	SetLimits(logger lager.Logger, containerRootFSPath string, limits garden.DiskLimits) error
	GetLimits(logger lager.Logger, containerRootFSPath string) (garden.DiskLimits, error)
	GetUsage(logger lager.Logger, containerRootFSPath string) (garden.ContainerDiskStat, error)

	Setup() error

	IsEnabled() bool
}

//go:generate counterfeiter -o fake_network_statisticser/fake_network_statisticser.go . NetworkStatisticser
type NetworkStatisticser interface {
	Statistics() (stats garden.ContainerNetworkStat, err error)
}

type BandwidthManager interface {
	SetLimits(lager.Logger, garden.BandwidthLimits) error
	GetLimits(lager.Logger) (garden.ContainerBandwidthStat, error)
}

type CgroupsManager interface {
	Set(subsystem, name, value string) error
	Get(subsystem, name string) (string, error)
	SubsystemPath(subsystem string) (string, error)
}

type LinuxContainer struct {
	propertiesMutex sync.RWMutex
	stateMutex      sync.RWMutex
	eventsMutex     sync.RWMutex
	bandwidthMutex  sync.RWMutex
	diskMutex       sync.RWMutex
	memoryMutex     sync.RWMutex
	cpuMutex        sync.RWMutex
	netInsMutex     sync.RWMutex
	netOutsMutex    sync.RWMutex
	linux_backend.LinuxContainerSpec

	portPool         PortPool
	runner           command_runner.CommandRunner
	cgroupsManager   CgroupsManager
	quotaManager     QuotaManager
	bandwidthManager BandwidthManager
	processTracker   process_tracker.ProcessTracker
	filter           network.Filter
	processIDPool    *ProcessIDPool

	oomNotifier *exec.Cmd
	oomMutex    sync.RWMutex

	mtu uint32

	netStats NetworkStatisticser

	logger lager.Logger
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

type PortPool interface {
	Acquire() (uint32, error)
	Remove(uint32) error
	Release(uint32)
}

func NewLinuxContainer(
	spec linux_backend.LinuxContainerSpec,
	portPool PortPool,
	runner command_runner.CommandRunner,
	cgroupsManager CgroupsManager,
	quotaManager QuotaManager,
	bandwidthManager BandwidthManager,
	processTracker process_tracker.ProcessTracker,
	filter network.Filter,
	netStats NetworkStatisticser,
	logger lager.Logger,
) *LinuxContainer {
	return &LinuxContainer{
		LinuxContainerSpec: spec,

		portPool:         portPool,
		runner:           runner,
		cgroupsManager:   cgroupsManager,
		quotaManager:     quotaManager,
		bandwidthManager: bandwidthManager,
		processTracker:   processTracker,
		filter:           filter,
		processIDPool:    &ProcessIDPool{},
		netStats:         netStats,

		logger: logger,
	}
}

func (c *LinuxContainer) ID() string {
	return c.LinuxContainerSpec.ID
}

func (c *LinuxContainer) ResourceSpec() linux_backend.LinuxContainerSpec {
	return c.LinuxContainerSpec
}

func (c *LinuxContainer) RootFSPath() string {
	return c.ContainerRootFSPath
}

func (c *LinuxContainer) Handle() string {
	return c.LinuxContainerSpec.Handle
}

func (c *LinuxContainer) GraceTime() time.Duration {
	return c.LinuxContainerSpec.GraceTime
}

func (c *LinuxContainer) State() linux_backend.State {
	c.stateMutex.RLock()
	defer c.stateMutex.RUnlock()

	return c.LinuxContainerSpec.State
}

func (c *LinuxContainer) Events() []string {
	c.eventsMutex.RLock()
	defer c.eventsMutex.RUnlock()

	events := make([]string, len(c.LinuxContainerSpec.Events))
	copy(events, c.LinuxContainerSpec.Events)
	return events
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

	processSnapshots := []linux_backend.ActiveProcess{}

	for _, p := range c.processTracker.ActiveProcesses() {
		processSnapshots = append(processSnapshots, linux_backend.ActiveProcess{ID: p.ID()})
	}

	properties, _ := c.Properties()

	snapshot := ContainerSnapshot{
		ID:         c.ID(),
		Handle:     c.Handle(),
		RootFSPath: c.RootFSPath(),

		GraceTime: c.LinuxContainerSpec.GraceTime,

		State:  string(c.State()),
		Events: c.Events(),

		Limits: linux_backend.Limits{
			Bandwidth: c.LinuxContainerSpec.Limits.Bandwidth,
			CPU:       c.LinuxContainerSpec.Limits.CPU,
			Disk:      c.LinuxContainerSpec.Limits.Disk,
			Memory:    c.LinuxContainerSpec.Limits.Memory,
		},

		Resources: ResourcesSnapshot{
			UserUID: c.Resources.UserUID,
			RootUID: c.Resources.RootUID,
			Network: c.Resources.Network,
			Bridge:  c.Resources.Bridge,
			Ports:   c.Resources.Ports,
		},

		NetIns:  c.NetIns,
		NetOuts: c.NetOuts,

		Processes: processSnapshots,

		Properties: properties,

		EnvVars: c.Env,
	}

	var err error

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

func (c *LinuxContainer) Restore(snapshot linux_backend.LinuxContainerSpec) error {
	cLog := c.logger.Session("restore")

	cLog.Debug("restoring")

	cRunner := logging.Runner{
		CommandRunner: c.runner,
		Logger:        cLog,
	}

	c.setState(linux_backend.State(snapshot.State))

	c.Env = snapshot.Env

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

		pidfile := path.Join(c.ContainerPath, "processes", fmt.Sprintf("%d.pid", process.ID))

		signaller := &linux_backend.NamespacedSignaller{
			Runner:        c.runner,
			ContainerPath: c.ContainerPath,
			PidFilePath:   pidfile,
			Logger:        c.logger,
		}

		c.processTracker.Restore(process.ID, signaller)
	}

	net := exec.Command(path.Join(c.ContainerPath, "net.sh"), "setup")

	if err := cRunner.Run(net); err != nil {
		cLog.Error("failed-to-reenforce-network-rules", err)
		return err
	}

	for _, in := range snapshot.NetIns {
		if _, _, err := c.NetIn(in.HostPort, in.ContainerPort); err != nil {
			cLog.Error("failed-to-reenforce-port-mapping", err)
			return err
		}
	}

	for _, out := range snapshot.NetOuts {
		if err := c.NetOut(out); err != nil {
			cLog.Error("failed-to-reenforce-net-out", err)
			return err
		}
	}

	cLog.Info("restored")

	return nil
}

func (c *LinuxContainer) Start() error {
	cLog := c.logger.Session("start")

	cLog.Debug("starting")

	start := exec.Command(path.Join(c.ContainerPath, "start.sh"))
	start.Env = []string{
		"id=" + c.ID(),
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

	c.setState(linux_backend.StateActive)

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
	stop := exec.Command(path.Join(c.ContainerPath, "stop.sh"))

	if kill {
		stop.Args = append(stop.Args, "-w", "0")
	}

	err := c.runner.Run(stop)
	if err != nil {
		return err
	}

	c.stopOomNotifier()

	c.setState(linux_backend.StateStopped)

	return nil
}

func (c *LinuxContainer) Properties() (garden.Properties, error) {
	c.propertiesMutex.RLock()
	defer c.propertiesMutex.RUnlock()

	return c.LinuxContainerSpec.Properties, nil
}

func (c *LinuxContainer) Property(key string) (string, error) {
	c.propertiesMutex.RLock()
	defer c.propertiesMutex.RUnlock()

	value, found := c.LinuxContainerSpec.Properties[key]
	if !found {
		return "", UndefinedPropertyError{key}
	}

	return value, nil
}

func (c *LinuxContainer) SetProperty(key string, value string) error {
	c.propertiesMutex.Lock()
	defer c.propertiesMutex.Unlock()

	props := garden.Properties{}
	for k, v := range c.LinuxContainerSpec.Properties {
		props[k] = v
	}

	props[key] = value

	c.LinuxContainerSpec.Properties = props

	return nil
}

func (c *LinuxContainer) RemoveProperty(key string) error {
	c.propertiesMutex.Lock()
	defer c.propertiesMutex.Unlock()

	if _, found := c.LinuxContainerSpec.Properties[key]; !found {
		return UndefinedPropertyError{key}
	}

	delete(c.LinuxContainerSpec.Properties, key)

	return nil
}

func (c *LinuxContainer) HasProperties(properties garden.Properties) bool {
	c.propertiesMutex.RLock()
	defer c.propertiesMutex.RUnlock()

	for k, v := range properties {
		if value, ok := c.LinuxContainerSpec.Properties[k]; !ok || (ok && value != v) {
			return false
		}
	}

	return true
}

func (c *LinuxContainer) Info() (garden.ContainerInfo, error) {
	mappedPorts := []garden.PortMapping{}

	c.netInsMutex.RLock()

	for _, spec := range c.NetIns {
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

	properties, _ := c.Properties()

	info := garden.ContainerInfo{
		State:         string(c.State()),
		Events:        c.Events(),
		Properties:    properties,
		ContainerPath: c.ContainerPath,
		ProcessIDs:    processIDs,
		MappedPorts:   mappedPorts,
	}

	info.ContainerIP = c.Resources.Network.IP.String()
	info.HostIP = subnets.GatewayIP(c.Resources.Network.Subnet).String()
	info.ExternalIP = c.Resources.ExternalIP.String()

	return info, nil
}

func (c *LinuxContainer) StreamIn(spec garden.StreamInSpec) error {
	nsTarPath := path.Join(c.ContainerPath, "bin", "nstar")
	pidPath := path.Join(c.ContainerPath, "run", "wshd.pid")

	pidFile, err := os.Open(pidPath)
	if err != nil {
		return err
	}

	var pid int
	_, err = fmt.Fscanf(pidFile, "%d", &pid)
	if err != nil {
		return err
	}

	user := spec.User
	if user == "" {
		user = "root"
	}

	tar := exec.Command(
		nsTarPath,
		strconv.Itoa(pid),
		user,
		spec.Path,
	)

	tar.Stdin = spec.TarStream

	cLog := c.logger.Session("stream-in")

	cRunner := logging.Runner{
		CommandRunner: c.runner,
		Logger:        cLog,
	}

	return cRunner.Run(tar)
}

func (c *LinuxContainer) StreamOut(spec garden.StreamOutSpec) (io.ReadCloser, error) {
	workingDir := filepath.Dir(spec.Path)
	compressArg := filepath.Base(spec.Path)
	if strings.HasSuffix(spec.Path, "/") {
		workingDir = spec.Path
		compressArg = "."
	}

	nsTarPath := path.Join(c.ContainerPath, "bin", "nstar")
	pidPath := path.Join(c.ContainerPath, "run", "wshd.pid")

	pidFile, err := os.Open(pidPath)
	if err != nil {
		return nil, err
	}

	var pid int
	_, err = fmt.Fscanf(pidFile, "%d", &pid)
	if err != nil {
		return nil, err
	}

	user := spec.User
	if user == "" {
		user = "root"
	}

	tar := exec.Command(
		nsTarPath,
		strconv.Itoa(pid),
		user,
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

func (c *LinuxContainer) NetIn(hostPort uint32, containerPort uint32) (uint32, uint32, error) {
	if hostPort == 0 {
		randomPort, err := c.portPool.Acquire()
		if err != nil {
			return 0, 0, err
		}

		c.Resources.AddPort(randomPort)

		hostPort = randomPort
	}

	if containerPort == 0 {
		containerPort = hostPort
	}

	net := exec.Command(path.Join(c.ContainerPath, "net.sh"), "in")
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

	c.NetIns = append(c.NetIns, linux_backend.NetInSpec{hostPort, containerPort})

	return hostPort, containerPort, nil
}

func (c *LinuxContainer) NetOut(r garden.NetOutRule) error {
	err := c.filter.NetOut(r)
	if err != nil {
		return err
	}

	c.netOutsMutex.Lock()
	defer c.netOutsMutex.Unlock()

	c.NetOuts = append(c.NetOuts, r)

	return nil
}

func (c *LinuxContainer) setState(state linux_backend.State) {
	c.stateMutex.Lock()
	defer c.stateMutex.Unlock()

	c.LinuxContainerSpec.State = state
}

func (c *LinuxContainer) registerEvent(event string) {
	c.eventsMutex.Lock()
	defer c.eventsMutex.Unlock()

	c.LinuxContainerSpec.Events = append(c.LinuxContainerSpec.Events, event)
}
