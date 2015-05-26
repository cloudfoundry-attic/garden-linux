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
	"github.com/cloudfoundry-incubator/garden-linux/network"
	"github.com/cloudfoundry-incubator/garden-linux/network/subnets"
	"github.com/cloudfoundry-incubator/garden-linux/old/bandwidth_manager"
	"github.com/cloudfoundry-incubator/garden-linux/old/cgroups_manager"
	"github.com/cloudfoundry-incubator/garden-linux/old/logging"
	"github.com/cloudfoundry-incubator/garden-linux/old/quota_manager"
	"github.com/cloudfoundry-incubator/garden-linux/process"
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

type LinuxContainer struct {
	logger lager.Logger

	id         string
	handle     string
	path       string
	rootFSPath string

	properties      garden.Properties
	propertiesMutex sync.RWMutex

	graceTime time.Duration

	state      State
	stateMutex sync.RWMutex

	events      []string
	eventsMutex sync.RWMutex

	resources *linux_backend.Resources

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

	netOuts      []garden.NetOutRule
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
	id, handle, path, rootFSPath string,
	properties garden.Properties,
	graceTime time.Duration,
	resources *linux_backend.Resources,
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

		id:         id,
		handle:     handle,
		path:       path,
		rootFSPath: rootFSPath,

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

func (c *LinuxContainer) Resources() *linux_backend.Resources {
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

	properties, _ := c.Properties()

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
			Network: c.resources.Network,
			Bridge:  c.resources.Bridge,
			Ports:   c.resources.Ports,
		},

		NetIns:  c.netIns,
		NetOuts: c.netOuts,

		Processes: processSnapshots,

		Properties: properties,

		EnvVars: c.env.Array(),
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

		signaller := &linux_backend.NamespacedSignaller{
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

func (c *LinuxContainer) Properties() (garden.Properties, error) {
	c.propertiesMutex.RLock()
	defer c.propertiesMutex.RUnlock()

	return c.properties, nil
}

func (c *LinuxContainer) Property(key string) (string, error) {
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

	props := garden.Properties{}
	for k, v := range c.properties {
		props[k] = v
	}

	props[key] = value

	c.properties = props

	return nil
}

func (c *LinuxContainer) RemoveProperty(key string) error {
	c.propertiesMutex.Lock()
	defer c.propertiesMutex.Unlock()

	if _, found := c.properties[key]; !found {
		return UndefinedPropertyError{key}
	}

	delete(c.properties, key)

	return nil
}

func (c *LinuxContainer) HasProperties(properties garden.Properties) bool {
	c.propertiesMutex.RLock()
	defer c.propertiesMutex.RUnlock()

	for k, v := range properties {
		if value, ok := c.properties[k]; !ok || (ok && value != v) {
			return false
		}
	}

	return true
}

func (c *LinuxContainer) Info() (garden.ContainerInfo, error) {
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

	properties, _ := c.Properties()

	info := garden.ContainerInfo{
		State:         string(c.State()),
		Events:        c.Events(),
		Properties:    properties,
		ContainerPath: c.path,
		ProcessIDs:    processIDs,
		MappedPorts:   mappedPorts,
	}

	info.ContainerIP = c.resources.Network.IP.String()
	info.HostIP = subnets.GatewayIP(c.resources.Network.Subnet).String()
	info.ExternalIP = c.Resources().ExternalIP.String()

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

func (c *LinuxContainer) NetOut(r garden.NetOutRule) error {
	err := c.filter.NetOut(r)
	if err != nil {
		return err
	}

	c.netOutsMutex.Lock()
	defer c.netOutsMutex.Unlock()

	c.netOuts = append(c.netOuts, r)

	return nil
}

func (c *LinuxContainer) CurrentEnvVars() process.Env {
	return c.env
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
