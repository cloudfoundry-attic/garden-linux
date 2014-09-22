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
	"strings"
	"time"

	"github.com/cloudfoundry-incubator/garden/api"
	"github.com/cloudfoundry/gunk/command_runner"
	"github.com/pivotal-golang/lager"

	"github.com/cloudfoundry-incubator/garden-linux/linux_backend"
	"github.com/cloudfoundry-incubator/garden-linux/linux_backend/bandwidth_manager"
	"github.com/cloudfoundry-incubator/garden-linux/linux_backend/cgroups_manager"
	"github.com/cloudfoundry-incubator/garden-linux/linux_backend/container_pool/rootfs_provider"
	"github.com/cloudfoundry-incubator/garden-linux/linux_backend/network_pool"
	"github.com/cloudfoundry-incubator/garden-linux/linux_backend/process_tracker"
	"github.com/cloudfoundry-incubator/garden-linux/linux_backend/quota_manager"
	"github.com/cloudfoundry-incubator/garden-linux/linux_backend/uid_pool"
	"github.com/cloudfoundry-incubator/garden-linux/logging"
	"github.com/cloudfoundry-incubator/garden-linux/sysconfig"
)

var ErrUnknownRootFSProvider = errors.New("unknown rootfs provider")

type LinuxContainerPool struct {
	logger lager.Logger

	binPath   string
	depotPath string

	sysconfig sysconfig.Config

	denyNetworks  []string
	allowNetworks []string

	rootfsProviders map[string]rootfs_provider.RootFSProvider

	uidPool     uid_pool.UIDPool
	networkPool network_pool.NetworkPool
	portPool    linux_backend.PortPool

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
	networkPool network_pool.NetworkPool,
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
		networkPool: networkPool,
		portPool:    portPool,

		runner: runner,

		quotaManager: quotaManager,

		containerIDs: make(chan string),
	}

	go pool.generateContainerIDs()

	return pool
}

func (p *LinuxContainerPool) MaxContainers() int {
	maxNet := p.networkPool.InitialSize()
	maxUid := p.uidPool.InitialSize()
	if maxNet < maxUid {
		return maxNet
	}
	return maxUid
}

func (p *LinuxContainerPool) Setup() error {
	setup := exec.Command(path.Join(p.binPath, "setup.sh"), "1>/var/log/setup.log", "2>&1")
	setup.Env = []string{
		"DEBUG=true",
		"POOL_NETWORK=" + p.networkPool.Network().String(),
		"DENY_NETWORKS=" + formatNetworks(p.denyNetworks),
		"ALLOW_NETWORKS=" + formatNetworks(p.allowNetworks),
		"CONTAINER_DEPOT_PATH=" + p.depotPath,
		"CONTAINER_DEPOT_MOUNT_POINT_PATH=" + p.quotaManager.MountPoint(),
		fmt.Sprintf("DISK_QUOTA_ENABLED=%v", p.quotaManager.IsEnabled()),
		"PATH=" + os.Getenv("PATH"),
	}

	err := p.runner.Run(setup) // TODO: output does not appear
	if err != nil {
		p.logger.Error("setup.sh-failed", err, lager.Data{
			"p.binPath": p.binPath,
			"Env":       setup.Env,
		})
		return err
	}

	return nil
}

func formatNetworks(networks []string) string {
	return strings.Join(networks, " ")
}

func (p *LinuxContainerPool) Prune(keep map[string]bool) error {
	entries, err := ioutil.ReadDir(p.depotPath)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		id := entry.Name()
		if id == "tmp" {
			continue
		}

		_, found := keep[id]
		if found {
			continue
		}

		pLog := p.logger.Session("prune", lager.Data{
			"id": id,
		})

		pLog.Info("pruning")

		err = p.releaseSystemResources(pLog, id)
		if err != nil {
			return err
		}
	}

	return nil
}

func (p *LinuxContainerPool) Create(spec api.ContainerSpec) (c linux_backend.Container, err error) {
	id := <-p.containerIDs
	containerPath := path.Join(p.depotPath, id)
	pLog := p.logger.Session(id)

	pLog.Info("creating")

	resources, err := p.aquirePoolResources()
	if err != nil {
		return nil, err
	}
	defer cleanup(&err, func() {
		p.releasePoolResources(resources)
	})

	rootFSEnvVars, err := p.aquireSystemResources(id, containerPath, spec.RootFSPath, resources, spec.BindMounts, pLog)
	if err != nil {
		return nil, err
	}

	pLog.Info("created")

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
		mergeEnv(spec.Env, rootFSEnvVars),
	), nil
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

	err = p.uidPool.Remove(resources.UID)
	if err != nil {
		return nil, err
	}

	err = p.networkPool.Remove(resources.Network)
	if err != nil {
		p.uidPool.Release(resources.UID)
		return nil, err
	}

	for _, port := range resources.Ports {
		err = p.portPool.Remove(port)
		if err != nil {
			p.uidPool.Release(resources.UID)
			p.networkPool.Release(resources.Network)

			for _, port := range resources.Ports {
				p.portPool.Release(port)
			}

			return nil, err
		}
	}

	containerPath := path.Join(p.depotPath, id)

	cgroupsManager := cgroups_manager.New(p.sysconfig.CgroupPath, id)

	bandwidthManager := bandwidth_manager.New(containerPath, id, p.runner)

	container := linux_backend.NewLinuxContainer(
		p.logger.Session(id),
		id,
		containerSnapshot.Handle,
		containerPath,
		containerSnapshot.Properties,
		containerSnapshot.GraceTime,
		linux_backend.NewResources(
			resources.UID,
			resources.Network,
			resources.Ports,
		),
		p.portPool,
		p.runner,
		cgroupsManager,
		p.quotaManager,
		bandwidthManager,
		process_tracker.New(containerPath, p.runner),
		containerSnapshot.EnvVars,
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

	err := p.releaseSystemResources(pLog, container.ID())
	if err != nil {
		return err
	}

	linuxContainer := container.(*linux_backend.LinuxContainer)
	p.releasePoolResources(linuxContainer.Resources())

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
	bindMounts []api.BindMount) error {
	hook := path.Join(containerPath, "lib", "hook-child-before-pivot.sh")

	for _, bm := range bindMounts {
		dstMount := path.Join(rootfsPath, bm.DstPath)
		srcPath := bm.SrcPath

		if bm.Origin == api.BindMountOriginContainer {
			srcPath = path.Join(rootfsPath, srcPath)
		}

		mode := "ro"
		if bm.Mode == api.BindMountModeRW {
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

func (p *LinuxContainerPool) aquirePoolResources() (*linux_backend.Resources, error) {
	var err error
	resources := linux_backend.NewResources(0, nil, nil)

	resources.UID, err = p.uidPool.Acquire()
	if err != nil {
		p.logger.Error("uid-acquire-failed", err)
		return nil, err
	}

	resources.Network, err = p.networkPool.Acquire()
	if err != nil {
		p.logger.Error("network-acquire-failed", err)
		p.releasePoolResources(resources)
		return nil, err
	}

	return resources, nil
}

func (p *LinuxContainerPool) releasePoolResources(resources *linux_backend.Resources) {
	for _, port := range resources.Ports {
		p.portPool.Release(port)
	}

	if resources.UID != 0 {
		p.uidPool.Release(resources.UID)
	}

	if resources.Network != nil {
		p.networkPool.Release(resources.Network)
	}
}

func (p *LinuxContainerPool) aquireSystemResources(id, containerPath, rootFSPath string, resources *linux_backend.Resources, bindMounts []api.BindMount, pLog lager.Logger) ([]string, error) {
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
	create.Env = []string{
		"id=" + id,
		"rootfs_path=" + rootfsPath,
		fmt.Sprintf("user_uid=%d", resources.UID),
		fmt.Sprintf("network_host_ip=%s", resources.Network.HostIP()),
		fmt.Sprintf("network_container_ip=%s", resources.Network.ContainerIP()),
		"PATH=" + os.Getenv("PATH"),
	}

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

	return provider.CleanupRootFS(logger, id)
}

func getHandle(handle, id string) string {
	if handle != "" {
		return handle
	}
	return id
}

func mergeEnv(env1, env2 []string) []string {
	for _, entry := range env2 {
		env1 = append(env1, entry)
	}
	return env1
}

func cleanup(err *error, undo func()) {
	if *err != nil {
		undo()
	}
}
