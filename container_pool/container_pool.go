package container_pool

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"os/exec"
	"path"
	"strconv"
	"time"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry/gunk/command_runner"
	"github.com/pivotal-golang/lager"

	"github.com/cloudfoundry-incubator/garden-linux/network"
	"github.com/cloudfoundry-incubator/garden-linux/network/cnet"
	"github.com/cloudfoundry-incubator/garden-linux/network/iptables"
	"github.com/cloudfoundry-incubator/garden-linux/old/linux_backend"
	"github.com/cloudfoundry-incubator/garden-linux/old/linux_backend/bandwidth_manager"
	"github.com/cloudfoundry-incubator/garden-linux/old/linux_backend/cgroups_manager"
	"github.com/cloudfoundry-incubator/garden-linux/old/linux_backend/container_pool/rootfs_provider"
	"github.com/cloudfoundry-incubator/garden-linux/old/linux_backend/quota_manager"
	"github.com/cloudfoundry-incubator/garden-linux/old/linux_backend/uid_pool"
	"github.com/cloudfoundry-incubator/garden-linux/old/logging"
	"github.com/cloudfoundry-incubator/garden-linux/old/sysconfig"
	"github.com/cloudfoundry-incubator/garden-linux/process"
	"github.com/cloudfoundry-incubator/garden-linux/process_tracker"
)

var ErrUnknownRootFSProvider = errors.New("unknown rootfs provider")
var ErrNetworkHostbitsNonZero = errors.New("network host bits non-zero")

//go:generate counterfeiter -o fake_container_pool/FakeFilterProvider.go . FilterProvider
type FilterProvider interface {
	ProvideFilter(containerId string) network.Filter
}

type LinuxContainerPool struct {
	logger lager.Logger

	binPath   string
	depotPath string

	sysconfig sysconfig.Config

	denyNetworks  []string
	allowNetworks []string

	rootfsProviders map[string]rootfs_provider.RootFSProvider

	uidPool     uid_pool.UIDPool
	cnBuilder   cnet.Builder
	cnPersistor CNPersistor
	portPool    linux_backend.PortPool

	filterProvider FilterProvider
	defaultChain   iptables.Chain

	runner command_runner.CommandRunner

	quotaManager quota_manager.QuotaManager

	containerIDs chan string
}

func New(
	logger lager.Logger,
	binPath, depotPath string,
	sysconfig sysconfig.Config,
	rootfsProviders map[string]rootfs_provider.RootFSProvider,
	uidPool uid_pool.UIDPool,
	cnBuilder cnet.Builder,
	cnPersistor CNPersistor,
	filterProvider FilterProvider,
	defaultChain iptables.Chain,
	portPool linux_backend.PortPool,
	denyNetworks, allowNetworks []string,
	runner command_runner.CommandRunner,
	quotaManager quota_manager.QuotaManager,
) *LinuxContainerPool {
	pool := &LinuxContainerPool{
		logger: logger.Session("pool"),

		binPath:   binPath,
		depotPath: depotPath,

		sysconfig: sysconfig,

		rootfsProviders: rootfsProviders,

		allowNetworks: allowNetworks,
		denyNetworks:  denyNetworks,

		uidPool:     uidPool,
		cnBuilder:   cnBuilder,
		cnPersistor: cnPersistor,

		filterProvider: filterProvider,
		defaultChain:   defaultChain,

		portPool: portPool,

		runner: runner,

		quotaManager: quotaManager,

		containerIDs: make(chan string),
	}

	go pool.generateContainerIDs()

	return pool
}

func (p *LinuxContainerPool) MaxContainers() int {
	maxNet := p.cnBuilder.Capacity()
	maxUid := p.uidPool.InitialSize()
	if maxNet < maxUid {
		return maxNet
	}
	return maxUid
}

func (p *LinuxContainerPool) Setup() error {
	setup := exec.Command(path.Join(p.binPath, "setup.sh"))
	setup.Env = []string{
		"CONTAINER_DEPOT_PATH=" + p.depotPath,
		"CONTAINER_DEPOT_MOUNT_POINT_PATH=" + p.quotaManager.MountPoint(),
		fmt.Sprintf("DISK_QUOTA_ENABLED=%v", p.quotaManager.IsEnabled()),
		"PATH=" + os.Getenv("PATH"),
	}

	err := p.runner.Run(setup)
	if err != nil {
		return err
	}

	return p.setupIPTables()
}

func (p *LinuxContainerPool) setupIPTables() error {
	for _, n := range p.allowNetworks {
		if n == "" {
			continue
		}

		if err := p.defaultChain.AppendRule("", n, iptables.Return); err != nil {
			return fmt.Errorf("container_pool: setting up allow rules in iptables: %v", err)
		}
	}

	for _, n := range p.denyNetworks {
		if n == "" {
			continue
		}

		if err := p.defaultChain.AppendRule("", n, iptables.Reject); err != nil {
			return fmt.Errorf("container_pool: setting up deny rules in iptables: %v", err)
		}
	}

	return nil
}

func (p *LinuxContainerPool) Prune(keep map[string]bool) error {
	entries, err := ioutil.ReadDir(p.depotPath)
	if err != nil {
		p.logger.Error("prune-container-pool-path-error", err, lager.Data{"depotPath": p.depotPath})
		return fmt.Errorf("Cannot read path %q: %s", p.depotPath, err)
	}

	for _, entry := range entries {
		id := entry.Name()
		if id == "tmp" { // ignore temporary directory in depotPath
			continue
		}

		_, found := keep[id]
		if found {
			continue
		}

		p.pruneEntry(id)
	}

	return nil
}

// pruneEntry does not report errors, only log them
func (p *LinuxContainerPool) pruneEntry(id string) {
	pLog := p.logger.Session("prune", lager.Data{"id": id})

	pLog.Info("prune")

	containerPath := path.Join(p.depotPath, id)
	cn, err := p.cnPersistor.Recover(containerPath)
	if err != nil {
		pLog.Error("cnet-recovery-error", err)
	} else {
		err = p.cnBuilder.Dismantle(cn)
		if err != nil {
			pLog.Error("cnet-dismantle-error", err)
		}
	}

	err = p.releaseSystemResources(pLog, id)
	if err != nil {
		pLog.Error("release-system-resources-error", err)
	}

	pLog.Info("end of prune")
}

func (p *LinuxContainerPool) Create(spec garden.ContainerSpec) (c linux_backend.Container, err error) {
	id := <-p.containerIDs
	containerPath := path.Join(p.depotPath, id)
	pLog := p.logger.Session(id)

	pLog.Info("creating")

	resources, err := p.acquirePoolResources(spec, id)
	if err != nil {
		return nil, err
	}
	defer cleanup(&err, func() {
		p.releasePoolResources(resources)
	})

	err = p.cnPersistor.Persist(resources.Network, containerPath)
	if err != nil {
		if releaseErr := p.releaseSystemResources(pLog, id); releaseErr != nil {
			pLog.Error("failed-to-release-system-resources", releaseErr)
		}
		return nil, err
	}

	rootFSEnv, err := p.acquireSystemResources(id, containerPath, spec.RootFSPath, resources, spec.BindMounts, pLog)
	if err != nil {
		return nil, err
	}

	pLog.Info("created")

	specEnv, err := process.NewEnv(spec.Env)
	if err != nil {
		return nil, err
	}

	pLog.Debug("calculate-environment", lager.Data{
		"rootfs-env": rootFSEnv,
		"create-env": specEnv,
	})
	return linux_backend.NewLinuxContainer(
		pLog,
		id,
		getHandle(spec.Handle, id),
		containerPath,
		spec.Properties,
		spec.GraceTime,
		resources,
		p.portPool,
		p.runner,
		cgroups_manager.New(p.sysconfig.CgroupPath, id),
		p.quotaManager,
		bandwidth_manager.New(containerPath, id, p.runner),
		process_tracker.New(containerPath, p.runner),
		rootFSEnv.Merge(specEnv),
		p.filterProvider.ProvideFilter(id),
	), nil
}

func (p *LinuxContainerPool) releaseUIDs(userUID, rootUID uint32) {
	if userUID != 0 {
		p.uidPool.Release(userUID)
	}

	if rootUID != 0 {
		p.uidPool.Release(rootUID)
	}
}

func (p *LinuxContainerPool) Restore(snapshot io.Reader) (linux_backend.Container, error) {
	var containerSnapshot linux_backend.ContainerSnapshot

	err := json.NewDecoder(snapshot).Decode(&containerSnapshot)
	if err != nil {
		return nil, err
	}

	id := containerSnapshot.ID

	rLog := p.logger.Session("restore", lager.Data{
		"id": id,
	})

	rLog.Debug("restoring")

	resources := containerSnapshot.Resources

	err = p.uidPool.Remove(resources.UserUID)
	if err != nil {
		return nil, err
	}

	if resources.RootUID != 0 {
		err = p.uidPool.Remove(resources.RootUID)
		if err != nil {
			return nil, err
		}
	}

	state, err := p.cnBuilder.Rebuild(resources.Network)
	if err != nil {
		p.releaseUIDs(resources.UserUID, resources.RootUID)
		return nil, err
	}

	for _, port := range resources.Ports {
		err = p.portPool.Remove(port)
		if err != nil {
			p.releaseUIDs(resources.UserUID, resources.RootUID)
			p.cnBuilder.Dismantle(state)

			for _, port := range resources.Ports {
				p.portPool.Release(port)
			}

			return nil, err
		}
	}

	containerPath := path.Join(p.depotPath, id)

	cgroupsManager := cgroups_manager.New(p.sysconfig.CgroupPath, id)

	bandwidthManager := bandwidth_manager.New(containerPath, id, p.runner)

	containerLogger := p.logger.Session(id)

	containerEnv, err := process.NewEnv(containerSnapshot.EnvVars)
	if err != nil {
		return nil, err
	}

	container := linux_backend.NewLinuxContainer(
		containerLogger,
		id,
		containerSnapshot.Handle,
		containerPath,
		containerSnapshot.Properties,
		containerSnapshot.GraceTime,
		linux_backend.NewResources(
			resources.UserUID,
			resources.RootUID,
			state,
			resources.Ports,
			p.cnBuilder.ExternalIP(),
		),
		p.portPool,
		p.runner,
		cgroupsManager,
		p.quotaManager,
		bandwidthManager,
		process_tracker.New(containerPath, p.runner),
		containerEnv,
		p.filterProvider.ProvideFilter(id),
	)

	err = container.Restore(containerSnapshot)
	if err != nil {
		return nil, err
	}

	rLog.Info("restored")

	return container, nil
}

func (p *LinuxContainerPool) Destroy(container linux_backend.Container) error {
	pLog := p.logger.Session("destroy", lager.Data{
		"id": container.ID(),
	})

	pLog.Info("destroying")

	linuxContainer := container.(*linux_backend.LinuxContainer)
    resources := linuxContainer.Resources()
    if resources.Network != nil {
        err := p.cnBuilder.Dismantle(resources.Network)
        if err != nil {
            return err
        }
        resources.Network = nil
    }

	err := p.releaseSystemResources(pLog, container.ID())
	if err != nil {
		return err
	}

	p.releasePoolResources(resources)

	pLog.Info("destroyed")

	return nil
}

func (p *LinuxContainerPool) generateContainerIDs() string {
	for containerNum := time.Now().UnixNano(); ; containerNum++ {
		containerID := []byte{}

		var i uint
		for i = 0; i < 11; i++ {
			containerID = strconv.AppendInt(
				containerID,
				(containerNum>>(55-(i+1)*5))&31,
				32,
			)
		}

		p.containerIDs <- string(containerID)
	}
}

func (p *LinuxContainerPool) writeBindMounts(containerPath string,
	rootfsPath string,
	bindMounts []garden.BindMount) error {
	hook := path.Join(containerPath, "lib", "hook-parent-before-clone.sh")

	for _, bm := range bindMounts {
		dstMount := path.Join(rootfsPath, bm.DstPath)
		srcPath := bm.SrcPath

		if bm.Origin == garden.BindMountOriginContainer {
			srcPath = path.Join(rootfsPath, srcPath)
		}

		mode := "ro"
		if bm.Mode == garden.BindMountModeRW {
			mode = "rw"
		}

		linebreak := exec.Command("bash", "-c", "echo >> "+hook)
		err := p.runner.Run(linebreak)
		if err != nil {
			return err
		}

		mkdir := exec.Command("bash", "-c", "echo mkdir -p "+dstMount+" >> "+hook)
		err = p.runner.Run(mkdir)
		if err != nil {
			return err
		}

		mount := exec.Command("bash", "-c", "echo mount -n --bind "+srcPath+" "+dstMount+" >> "+hook)
		err = p.runner.Run(mount)
		if err != nil {
			return err
		}

		remount := exec.Command("bash", "-c", "echo mount -n --bind -o remount,"+mode+" "+srcPath+" "+dstMount+" >> "+hook)
		err = p.runner.Run(remount)
		if err != nil {
			return err
		}
	}

	return nil
}

func (p *LinuxContainerPool) saveRootFSProvider(id string, provider string) error {
	providerFile := path.Join(p.depotPath, id, "rootfs-provider")

	err := os.MkdirAll(path.Dir(providerFile), 0755)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(providerFile, []byte(provider), 0644)
}

func (p *LinuxContainerPool) acquirePoolResources(spec garden.ContainerSpec, id string) (*linux_backend.Resources, error) {
	var err error
	resources := linux_backend.NewResources(0, 1, nil, nil, p.cnBuilder.ExternalIP())

	if err := p.acquireUID(resources, spec.Privileged); err != nil {
		return nil, err
	}

	if resources.Network, err = p.cnBuilder.Build(spec.Network, &p.sysconfig, id); err != nil {
		p.logger.Error("network-acquire-failed", err)
		p.releasePoolResources(resources)
		return nil, err
	}

	return resources, nil
}

func (p *LinuxContainerPool) acquireUID(resources *linux_backend.Resources, privileged bool) error {
	var err error
	resources.UserUID, err = p.uidPool.Acquire()
	if err != nil {
		p.logger.Error("uid-acquire-failed", err)
		return err
	}

	resources.RootUID = 0
	if !privileged {
		resources.RootUID, err = p.uidPool.Acquire()
		if err != nil {
			p.logger.Error("uid-acquire-failed", err)
			return err
		}
	}

	return nil
}

func (p *LinuxContainerPool) releasePoolResources(resources *linux_backend.Resources) {
	for _, port := range resources.Ports {
		p.portPool.Release(port)
	}

	p.releaseUIDs(resources.UserUID, resources.RootUID)

	if resources.Network != nil {
		p.cnBuilder.Dismantle(resources.Network)
	}
}

func (p *LinuxContainerPool) acquireSystemResources(id, containerPath, rootFSPath string, resources *linux_backend.Resources, bindMounts []garden.BindMount, pLog lager.Logger) (process.Env, error) {
	rootfsURL, err := url.Parse(rootFSPath)
	if err != nil {
		pLog.Error("parse-rootfs-path-failed", err, lager.Data{
			"RootFSPath": rootFSPath,
		})
		return nil, err
	}

	provider, found := p.rootfsProviders[rootfsURL.Scheme]
	if !found {
		pLog.Error("unknown-rootfs-provider", nil, lager.Data{
			"provider": rootfsURL.Scheme,
		})
		return nil, ErrUnknownRootFSProvider
	}

	rootfsPath, rootFSEnvVars, err := provider.ProvideRootFS(pLog.Session("create-rootfs"), id, rootfsURL)
	if err != nil {
		pLog.Error("provide-rootfs-failed", err)
		return nil, err
	}

	createCmd := path.Join(p.binPath, "create.sh")
	create := exec.Command(createCmd, containerPath)
	env := process.Env{
		"id":          id,
		"rootfs_path": rootfsPath,
		"user_uid":    strconv.FormatUint(uint64(resources.UserUID), 10),
		"root_uid":    strconv.FormatUint(uint64(resources.RootUID), 10),
		"PATH":        os.Getenv("PATH"),
	}
	resources.Network.ConfigureEnvironment(env)
	p.cnBuilder.ConfigureEnvironment(env)
	create.Env = env.Array()

	pRunner := logging.Runner{
		CommandRunner: p.runner,
		Logger:        p.logger,
	}

	err = pRunner.Run(create)
	defer cleanup(&err, func() {
		p.tryReleaseSystemResources(p.logger, id)
	})

	if err != nil {
		p.logger.Error("create-command-failed", err, lager.Data{
			"CreateCmd": createCmd,
			"Env":       create.Env,
		})
		return nil, err
	}

	err = p.saveRootFSProvider(id, rootfsURL.Scheme)
	if err != nil {
		p.logger.Error("save-rootfs-provider-failed", err, lager.Data{
			"Id":     id,
			"rootfs": rootfsURL.String(),
		})
		return nil, err
	}

	err = p.writeBindMounts(containerPath, rootfsPath, bindMounts)
	if err != nil {
		p.logger.Error("bind-mounts-failed", err)
		return nil, err
	}

	if err = p.filterProvider.ProvideFilter(id).Setup(); err != nil {
		p.logger.Error("set-up-filter-failed", err)
		return nil, fmt.Errorf("container_pool: set up filter: %v", err)
	}

	return rootFSEnvVars, nil
}

func (p *LinuxContainerPool) tryReleaseSystemResources(logger lager.Logger, id string) {
	err := p.releaseSystemResources(logger, id)
	if err != nil {
		logger.Error("failed-to-undo-failed-create", err)
	}
}

func (p *LinuxContainerPool) releaseSystemResources(logger lager.Logger, id string) error {
	rootfsProvider, err := ioutil.ReadFile(path.Join(p.depotPath, id, "rootfs-provider"))
	if err != nil {
		rootfsProvider = []byte("")
	}

	pRunner := logging.Runner{
		CommandRunner: p.runner,
		Logger:        logger,
	}

	provider, found := p.rootfsProviders[string(rootfsProvider)]
	if !found {
		return ErrUnknownRootFSProvider
	}

	destroy := exec.Command(path.Join(p.binPath, "destroy.sh"), path.Join(p.depotPath, id))

	err = pRunner.Run(destroy)
	if err != nil {
		return err
	}

	if err = provider.CleanupRootFS(logger, id); err != nil {
		return err
	}

	p.filterProvider.ProvideFilter(id).TearDown()
	return nil
}

func getHandle(handle, id string) string {
	if handle != "" {
		return handle
	}
	return id
}

func cleanup(err *error, undo func()) {
	if *err != nil {
		undo()
	}
}
