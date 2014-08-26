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

	"github.com/cloudfoundry-incubator/garden/warden"
	"github.com/cloudfoundry/gunk/command_runner"
	"github.com/pivotal-golang/lager"

	"github.com/cloudfoundry-incubator/warden-linux/linux_backend"
	"github.com/cloudfoundry-incubator/warden-linux/linux_backend/bandwidth_manager"
	"github.com/cloudfoundry-incubator/warden-linux/linux_backend/cgroups_manager"
	"github.com/cloudfoundry-incubator/warden-linux/linux_backend/container_pool/rootfs_provider"
	"github.com/cloudfoundry-incubator/warden-linux/linux_backend/network_pool"
	"github.com/cloudfoundry-incubator/warden-linux/linux_backend/process_tracker"
	"github.com/cloudfoundry-incubator/warden-linux/linux_backend/quota_manager"
	"github.com/cloudfoundry-incubator/warden-linux/linux_backend/uid_pool"
	"github.com/cloudfoundry-incubator/warden-linux/logging"
	"github.com/cloudfoundry-incubator/warden-linux/sysconfig"
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
	setup := exec.Command(path.Join(p.binPath, "setup.sh"))
	setup.Env = []string{
		"POOL_NETWORK=" + p.networkPool.Network().String(),
		"DENY_NETWORKS=" + formatNetworks(p.denyNetworks),
		"ALLOW_NETWORKS=" + formatNetworks(p.allowNetworks),
		"CONTAINER_DEPOT_PATH=" + p.depotPath,
		"CONTAINER_DEPOT_MOUNT_POINT_PATH=" + p.quotaManager.MountPoint(),
		fmt.Sprintf("DISK_QUOTA_ENABLED=%v", p.quotaManager.IsEnabled()),
		"PATH=" + os.Getenv("PATH"),
	}

	err := p.runner.Run(setup)
	if err != nil {
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

		err = p.destroy(pLog, id)
		if err != nil {
			return err
		}
	}

	return nil
}

func (p *LinuxContainerPool) Create(spec warden.ContainerSpec) (linux_backend.Container, error) {
	uid, err := p.uidPool.Acquire()
	if err != nil {
		p.logger.Error("uid-acquire-failed", err)
		return nil, err
	}

	network, err := p.networkPool.Acquire()
	if err != nil {
		p.uidPool.Release(uid)
		p.logger.Error("network-acquire-failed", err)
		return nil, err
	}

	id := <-p.containerIDs

	containerPath := path.Join(p.depotPath, id)

	cgroupsManager := cgroups_manager.New(p.sysconfig.CgroupPath, id)

	bandwidthManager := bandwidth_manager.New(containerPath, id, p.runner)

	handle := id
	if spec.Handle != "" {
		handle = spec.Handle
	}

	rootfsURL, err := url.Parse(spec.RootFSPath)
	if err != nil {
		p.logger.Error("parse-rootfs-path-failed", err, lager.Data{
			"RootFSPath": spec.RootFSPath,
		})
		return nil, err
	}

	provider, found := p.rootfsProviders[rootfsURL.Scheme]
	if !found {
		p.logger.Error("unknown-rootfs-provider", err, lager.Data{
			"provider": rootfsURL.Scheme,
		})
		return nil, ErrUnknownRootFSProvider
	}

	pLog := p.logger.Session(id)

	rootfsPath, rootFSEnvVars, err := provider.ProvideRootFS(pLog.Session("create-rootfs"), id, rootfsURL)
	if err != nil {
		p.logger.Error("provide-rootfs-failed", err)
		return nil, err
	}

	if len(rootFSEnvVars) > 0 {
		if spec.Env == nil {
			spec.Env = rootFSEnvVars
		} else {
			for i := range rootFSEnvVars {
				spec.Env = append(spec.Env, rootFSEnvVars[i])
			}
		}
	}

	container := linux_backend.NewLinuxContainer(
		pLog,
		id,
		handle,
		containerPath,
		spec.Properties,
		spec.GraceTime,
		linux_backend.NewResources(uid, network, []uint32{}),
		p.portPool,
		p.runner,
		cgroupsManager,
		p.quotaManager,
		bandwidthManager,
		process_tracker.New(containerPath, p.runner),
		spec.Env,
	)

	createCmd := path.Join(p.binPath, "create.sh")
	create := exec.Command(createCmd, containerPath)
	create.Env = []string{
		"id=" + container.ID(),
		"rootfs_path=" + rootfsPath,
		fmt.Sprintf("user_uid=%d", uid),
		fmt.Sprintf("network_host_ip=%s", network.HostIP()),
		fmt.Sprintf("network_container_ip=%s", network.ContainerIP()),

		"PATH=" + os.Getenv("PATH"),
	}

	pRunner := logging.Runner{
		CommandRunner: p.runner,
		Logger:        p.logger,
	}

	err = pRunner.Run(create)
	if err != nil {
		p.logger.Error("create-command-failed", err, lager.Data{
			"CreateCmd": createCmd,
			"Env":       create.Env,
		})
		p.uidPool.Release(uid)
		p.networkPool.Release(network)
		p.destroy(p.logger, container.ID())
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

	err = p.writeBindMounts(containerPath, spec.BindMounts)
	if err != nil {
		p.logger.Error("bind-mounts-failed", err)
		return nil, err
	}

	return container, nil
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

	err := p.destroy(pLog, container.ID())
	if err != nil {
		return err
	}

	linuxContainer := container.(*linux_backend.LinuxContainer)

	resources := linuxContainer.Resources()

	for _, port := range resources.Ports {
		p.portPool.Release(port)
	}

	p.uidPool.Release(resources.UID)

	p.networkPool.Release(resources.Network)

	return nil
}

func (p *LinuxContainerPool) destroy(logger lager.Logger, id string) error {
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

func (p *LinuxContainerPool) writeBindMounts(
	containerPath string,
	bindMounts []warden.BindMount,
) error {
	hook := path.Join(containerPath, "lib", "hook-child-before-pivot.sh")

	for _, bm := range bindMounts {
		dstMount := path.Join(containerPath, "mnt", bm.DstPath)
		srcPath := bm.SrcPath

		if bm.Origin == warden.BindMountOriginContainer {
			srcPath = path.Join(containerPath, "tmp", "rootfs", srcPath)
		}

		mode := "ro"
		if bm.Mode == warden.BindMountModeRW {
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
