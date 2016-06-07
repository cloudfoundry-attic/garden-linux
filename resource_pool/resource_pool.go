package resource_pool

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/blang/semver"
	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry/gunk/command_runner"
	"github.com/pivotal-golang/lager"

	"github.com/cloudfoundry-incubator/garden-linux/linux_backend"
	"github.com/cloudfoundry-incubator/garden-linux/linux_container"
	"github.com/cloudfoundry-incubator/garden-linux/logging"
	"github.com/cloudfoundry-incubator/garden-linux/network"
	"github.com/cloudfoundry-incubator/garden-linux/network/bridgemgr"
	"github.com/cloudfoundry-incubator/garden-linux/network/iptables"
	"github.com/cloudfoundry-incubator/garden-linux/network/subnets"
	"github.com/cloudfoundry-incubator/garden-linux/process"
	"github.com/cloudfoundry-incubator/garden-linux/sysconfig"
	"github.com/cloudfoundry-incubator/garden-shed/layercake"
	"github.com/cloudfoundry-incubator/garden-shed/rootfs_provider"
)

var (
	ErrUnknownRootFSProvider = errors.New("unknown rootfs provider")
)

//go:generate counterfeiter -o fake_filter_provider/FakeFilterProvider.go . FilterProvider
type FilterProvider interface {
	ProvideFilter(containerId string) network.Filter
}

//go:generate counterfeiter -o fake_subnet_pool/FakeSubnetPool.go . SubnetPool
type SubnetPool interface {
	Acquire(subnet subnets.SubnetSelector, ip subnets.IPSelector, logger lager.Logger) (*linux_backend.Network, error)
	Release(network *linux_backend.Network, logger lager.Logger) error
	Remove(network *linux_backend.Network, logger lager.Logger) error
	Capacity() int
}

//go:generate counterfeiter -o fake_rootfs_provider/FakeRootFSProvider.go . RootFSProvider
type RootFSProvider interface {
	Create(log lager.Logger, id string, spec rootfs_provider.Spec) (mountpoint string, envvar []string, err error)
	Destroy(log lager.Logger, id string) error
	GC(log lager.Logger) error
}

//go:generate counterfeiter -o fake_rootfs_cleaner/FakeRootFSCleaner.go . RootFSCleaner
type RootFSCleaner interface {
	Clean(log lager.Logger, path string) error
}

type Remover interface {
	Remove(id layercake.ID) error
}

//go:generate counterfeiter -o fake_mkdir_chowner/FakeMkdirChowner.go . MkdirChowner
type MkdirChowner interface {
	MkdirChown(path string, uid, gid uint32, mode os.FileMode) error
}

type LinuxResourcePool struct {
	logger lager.Logger

	binPath   string
	depotPath string

	sysconfig sysconfig.Config

	denyNetworks  []string
	allowNetworks []string

	rootFSProvider RootFSProvider
	rootFSCleaner  RootFSCleaner
	mappingList    rootfs_provider.MappingList

	subnetPool SubnetPool

	externalIP net.IP
	mtu        int

	portPool linux_container.PortPool

	bridges     bridgemgr.BridgeManager
	iptablesMgr linux_container.IPTablesManager

	filterProvider FilterProvider
	defaultChain   iptables.Chain

	runner command_runner.CommandRunner

	quotaManager linux_container.QuotaManager

	containerIDs chan string

	currentContainerVersion semver.Version

	mkdirChowner MkdirChowner
}

func New(
	logger lager.Logger,
	binPath, depotPath string,
	sysconfig sysconfig.Config,
	rootFSProvider RootFSProvider,
	rootFSCleaner RootFSCleaner,
	mappingList rootfs_provider.MappingList,
	externalIP net.IP,
	mtu int,
	subnetPool SubnetPool,
	bridges bridgemgr.BridgeManager,
	iptablesMgr linux_container.IPTablesManager,
	filterProvider FilterProvider,
	defaultChain iptables.Chain,
	portPool linux_container.PortPool,
	denyNetworks, allowNetworks []string,
	runner command_runner.CommandRunner,
	quotaManager linux_container.QuotaManager,
	currentContainerVersion semver.Version,
	mkdirChowner MkdirChowner,
) *LinuxResourcePool {
	pool := &LinuxResourcePool{
		logger: logger.Session("pool"),

		binPath:   binPath,
		depotPath: depotPath,

		sysconfig: sysconfig,

		rootFSProvider: rootFSProvider,
		rootFSCleaner:  rootFSCleaner,
		mappingList:    mappingList,

		allowNetworks: allowNetworks,
		denyNetworks:  denyNetworks,

		externalIP: externalIP,
		mtu:        mtu,

		subnetPool: subnetPool,

		bridges:     bridges,
		iptablesMgr: iptablesMgr,

		filterProvider: filterProvider,
		defaultChain:   defaultChain,

		portPool: portPool,

		runner: runner,

		quotaManager: quotaManager,

		containerIDs:            make(chan string),
		currentContainerVersion: currentContainerVersion,

		mkdirChowner: mkdirChowner,
	}

	go pool.generateContainerIDs()

	return pool
}

func (p *LinuxResourcePool) MaxContainers() int {
	return p.subnetPool.Capacity()
}

func (p *LinuxResourcePool) Setup() error {
	setup := exec.Command(path.Join(p.binPath, "setup.sh"))
	setup.Env = []string{
		"CONTAINER_DEPOT_PATH=" + p.depotPath,
		"PATH=" + os.Getenv("PATH"),
	}

	err := p.runner.Run(setup)
	if err != nil {
		return err
	}

	if err := p.quotaManager.Setup(); err != nil {
		return fmt.Errorf("resource_pool: enable disk quotas: %s", err)
	}

	return p.setupIPTables()
}

func (p *LinuxResourcePool) setupIPTables() error {
	for _, n := range p.allowNetworks {
		if n == "" {
			continue
		}

		if err := p.defaultChain.AppendRule("", n, iptables.Return); err != nil {
			return fmt.Errorf("resource_pool: setting up allow rules in iptables: %v", err)
		}
	}

	for _, n := range p.denyNetworks {
		if n == "" {
			continue
		}

		if err := p.defaultChain.AppendRule("", n, iptables.Reject); err != nil {
			return fmt.Errorf("resource_pool: setting up deny rules in iptables: %v", err)
		}
	}

	return nil
}

func (p *LinuxResourcePool) Prune(keep map[string]bool) error {
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

	if err := p.bridges.Prune(); err != nil {
		p.logger.Error("prune-bridges", err)
	}

	return nil
}

// pruneEntry does not report errors, only log them
func (p *LinuxResourcePool) pruneEntry(id string) {
	pLog := p.logger.Session("prune", lager.Data{"id": id})

	pLog.Info("prune")

	err := p.releaseSystemResources(pLog, id)
	if err != nil {
		pLog.Error("release-system-resources-error", err)
	}

	pLog.Info("end of prune")
}

func (p *LinuxResourcePool) Acquire(spec garden.ContainerSpec) (linux_backend.LinuxContainerSpec, error) {
	id := <-p.containerIDs
	containerPath := path.Join(p.depotPath, id)
	handle := getHandle(spec.Handle, id)
	pLog := p.logger.Session("acquire", lager.Data{"handle": handle, "id": id})

	iptablesCh := make(chan error, 1)

	go func(iptablesCh chan error) {
		pLog.Debug("setup-iptables-starting")
		if err := p.filterProvider.ProvideFilter(id).Setup(handle); err != nil {
			pLog.Error("setup-iptables-failed", err)
			iptablesCh <- fmt.Errorf("resource_pool: set up filter: %v", err)
		} else {
			pLog.Debug("setup-iptables-ended")
			iptablesCh <- nil
		}
	}(iptablesCh)

	pLog.Info("creating")

	resources, err := p.acquirePoolResources(spec, id, pLog)
	if err != nil {
		return linux_backend.LinuxContainerSpec{}, err
	}
	defer cleanup(&err, func() {
		p.releasePoolResources(resources, pLog)
	})

	pLog.Info("acquired-pool-resources")

	pLog.Info("running-graph-cleanup")
	if err := p.rootFSProvider.GC(pLog); err != nil {
		pLog.Error("graph-cleanup-failed", err)
	}

	containerRootFSPath, rootFSEnv, err := p.acquireSystemResources(
		spec, id, resources, pLog,
	)
	if err != nil {
		return linux_backend.LinuxContainerSpec{}, err
	}

	err = <-iptablesCh
	if err != nil {
		p.tryReleaseSystemResources(p.logger, id)
		return linux_backend.LinuxContainerSpec{}, err
	}

	pLog.Info("created")

	specEnv, err := process.NewEnv(spec.Env)
	if err != nil {
		p.tryReleaseSystemResources(p.logger, id)
		return linux_backend.LinuxContainerSpec{}, err
	}

	spec.Env = rootFSEnv.Merge(specEnv).Array()
	spec.Handle = handle

	return linux_backend.LinuxContainerSpec{
		ID:                  id,
		ContainerPath:       containerPath,
		ContainerRootFSPath: containerRootFSPath,
		Resources:           resources,
		Events:              []string{},
		Version:             p.currentContainerVersion,
		State:               linux_backend.StateBorn,

		ContainerSpec: spec,
	}, nil
}

func (p *LinuxResourcePool) Restore(snapshot io.Reader) (linux_backend.LinuxContainerSpec, error) {
	var containerSnapshot linux_container.ContainerSnapshot

	err := json.NewDecoder(snapshot).Decode(&containerSnapshot)
	if err != nil {
		return linux_backend.LinuxContainerSpec{}, err
	}

	id := containerSnapshot.ID
	rLog := p.logger.Session("restore", lager.Data{
		"handle": containerSnapshot.Handle,
		"id":     id,
	})

	rLog.Debug("restoring")

	resources := containerSnapshot.Resources
	subnetLogger := rLog.Session("subnet-pool")

	if err = p.subnetPool.Remove(resources.Network, subnetLogger); err != nil {
		return linux_backend.LinuxContainerSpec{}, err
	}

	if err = p.bridges.Rereserve(resources.Bridge, resources.Network.Subnet, id); err != nil {
		p.subnetPool.Release(resources.Network, subnetLogger)
		return linux_backend.LinuxContainerSpec{}, err
	}

	for _, port := range resources.Ports {
		err = p.portPool.Remove(port)
		if err != nil {
			p.subnetPool.Release(resources.Network, subnetLogger)

			for _, port := range resources.Ports {
				p.portPool.Release(port)
			}

			return linux_backend.LinuxContainerSpec{}, err
		}
	}

	version, err := p.restoreContainerVersion(id)
	if err != nil {
		return linux_backend.LinuxContainerSpec{}, err
	}

	spec := linux_backend.LinuxContainerSpec{
		ID:                  id,
		ContainerPath:       path.Join(p.depotPath, id),
		ContainerRootFSPath: containerSnapshot.RootFSPath,

		State:  linux_backend.State(containerSnapshot.State),
		Events: containerSnapshot.Events,
		ContainerSpec: garden.ContainerSpec{
			Handle:     containerSnapshot.Handle,
			GraceTime:  containerSnapshot.GraceTime,
			Properties: containerSnapshot.Properties,
		},

		Resources: linux_backend.NewResources(
			resources.RootUID,
			resources.Network,
			resources.Bridge,
			resources.Ports,
			p.externalIP,
		),

		Limits:    containerSnapshot.Limits,
		NetIns:    containerSnapshot.NetIns,
		NetOuts:   containerSnapshot.NetOuts,
		Processes: containerSnapshot.Processes,
		Version:   version,
	}

	return spec, nil
}

func (p *LinuxResourcePool) Release(container linux_backend.LinuxContainerSpec) error {
	pLog := p.logger.Session("release", lager.Data{
		"handle": container.Handle,
		"id":     container.ID,
	})

	pLog.Info("releasing")

	err := p.releaseSystemResources(pLog, container.ID)
	if err != nil {
		pLog.Error("release-system-resources", err)
		return err
	}

	p.releasePoolResources(container.Resources, pLog)

	pLog.Info("released")

	return nil
}

func (p *LinuxResourcePool) generateContainerIDs() {
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

func (p *LinuxResourcePool) writeBindMounts(containerPath string,
	rootFSPath string,
	bindMounts []garden.BindMount,
	mkdirUID int) error {
	hook := path.Join(containerPath, "lib", "hook-parent-before-clone.sh")

	for _, bm := range bindMounts {
		dstMount := path.Join(rootFSPath, bm.DstPath)
		srcPath := bm.SrcPath

		if bm.Origin == garden.BindMountOriginContainer {
			srcPath = path.Join(rootFSPath, srcPath)
		}

		mode := "ro"
		if bm.Mode == garden.BindMountModeRW {
			mode = "rw"
		}

		linebreak := exec.Command("bash", "-c", "echo >> "+hook)
		if err := p.runner.Run(linebreak); err != nil {
			return err
		}

		if err := p.mkdirChowner.MkdirChown(dstMount, uint32(mkdirUID), uint32(mkdirUID), 0755); err != nil {
			return err
		}

		mount := exec.Command("bash", "-c", "echo mount -n --bind "+srcPath+" "+dstMount+" >> "+hook)
		if err := p.runner.Run(mount); err != nil {
			return err
		}

		remount := exec.Command("bash", "-c", "echo mount -n --bind -o remount,"+mode+" "+srcPath+" "+dstMount+" >> "+hook)
		if err := p.runner.Run(remount); err != nil {
			return err
		}
	}

	return nil
}

func (p *LinuxResourcePool) saveBridgeName(id string, bridgeName string) error {
	bridgeNameFile := path.Join(p.depotPath, id, "bridge-name")
	return ioutil.WriteFile(bridgeNameFile, []byte(bridgeName), 0644)
}

func (p *LinuxResourcePool) saveRootFSProvider(id string, provider string) error {
	providerFile := path.Join(p.depotPath, id, "rootfs-provider")
	return ioutil.WriteFile(providerFile, []byte(provider), 0644)
}

func (p *LinuxResourcePool) saveContainerVersion(id string) error {
	versionFile := path.Join(p.depotPath, id, "version")
	return ioutil.WriteFile(versionFile, []byte(p.currentContainerVersion.String()), 0644)
}

func (p *LinuxResourcePool) restoreContainerVersion(id string) (semver.Version, error) {
	content, err := ioutil.ReadFile(filepath.Join(p.depotPath, id, "version"))
	if err != nil {
		if os.IsNotExist(err) {
			return linux_container.MissingVersion, nil
		}
		return semver.Version{}, err
	}

	return semver.Make(string(content))
}

func (p *LinuxResourcePool) acquirePoolResources(spec garden.ContainerSpec, id string, logger lager.Logger) (*linux_backend.Resources, error) {
	resources := linux_backend.NewResources(0, nil, "", nil, p.externalIP)

	subnet, ip, err := parseNetworkSpec(spec.Network)
	if err != nil {
		return nil, fmt.Errorf("create container: invalid network spec: %v", err)
	}

	if err := p.acquireUID(resources, spec.Privileged); err != nil {
		return nil, err
	}

	if resources.Network, err = p.subnetPool.Acquire(subnet, ip, logger.Session("subnet-pool")); err != nil {
		p.releasePoolResources(resources, logger)
		return nil, err
	}

	return resources, nil
}

func (p *LinuxResourcePool) acquireUID(resources *linux_backend.Resources, privileged bool) error {
	if !privileged {
		resources.RootUID = p.mappingList.Map(0)
		return nil
	}

	resources.RootUID = 0
	return nil
}

func (p *LinuxResourcePool) releasePoolResources(resources *linux_backend.Resources, logger lager.Logger) {
	for _, port := range resources.Ports {
		p.portPool.Release(port)
	}

	if resources.Network != nil {
		p.subnetPool.Release(resources.Network, logger.Session("subnet-pool"))
	}
}

func (p *LinuxResourcePool) acquireSystemResources(spec garden.ContainerSpec, id string, resources *linux_backend.Resources, pLog lager.Logger) (string, process.Env, error) {
	containerPath := path.Join(p.depotPath, id)
	if err := os.MkdirAll(containerPath, 0755); err != nil {
		return "", nil, fmt.Errorf("resource_pool: creating container directory: %v", err)
	}

	rootFSPath, rootFSEnvVars, err := p.setupContainerDirectories(spec, id, resources, pLog)
	if err != nil {
		os.RemoveAll(containerPath)
		return "", nil, err
	}

	createCmd := path.Join(p.binPath, "create.sh")
	create := exec.Command(createCmd, containerPath)
	suff, _ := resources.Network.Subnet.Mask.Size()
	env := process.Env{
		"id":                   id,
		"rootfs_path":          rootFSPath,
		"network_host_ip":      subnets.GatewayIP(resources.Network.Subnet).String(),
		"network_container_ip": resources.Network.IP.String(),
		"network_cidr_suffix":  strconv.Itoa(suff),
		"network_cidr":         resources.Network.Subnet.String(),
		"external_ip":          p.externalIP.String(),
		"container_iface_mtu":  fmt.Sprintf("%d", p.mtu),
		"bridge_iface":         resources.Bridge,
		"root_uid":             strconv.FormatUint(uint64(resources.RootUID), 10),
		"PATH":                 os.Getenv("PATH"),
	}
	create.Env = env.Array()

	pRunner := logging.Runner{
		CommandRunner: p.runner,
		Logger:        pLog.Session("create-script"),
	}

	err = pRunner.Run(create)
	defer cleanup(&err, func() {
		p.tryReleaseSystemResources(pLog, id)
	})

	if err != nil {
		pLog.Error("create-command-failed", err, lager.Data{
			"CreateCmd": createCmd,
			"Env":       create.Env,
		})
		return "", nil, err
	}

	err = p.saveRootFSProvider(id, "docker-composite")
	if err != nil {
		pLog.Error("save-rootfs-provider-failed", err, lager.Data{
			"Id":     id,
			"rootfs": spec.RootFSPath,
		})
		return "", nil, err
	}

	err = p.saveContainerVersion(id)
	if err != nil {
		pLog.Error("save-container-version-failed", err, lager.Data{
			"Id":            id,
			"ContainerPath": containerPath,
		})
		return "", nil, err
	}

	err = p.writeBindMounts(containerPath, rootFSPath, spec.BindMounts, resources.RootUID)
	if err != nil {
		pLog.Error("bind-mounts-failed", err)
		return "", nil, err
	}

	return rootFSPath, rootFSEnvVars, nil
}

func (p *LinuxResourcePool) setupRootfs(spec garden.ContainerSpec, id string, resources *linux_backend.Resources, pLog lager.Logger) (string, process.Env, error) {
	rootFSURL, err := url.Parse(spec.RootFSPath)
	if err != nil {
		pLog.Error("parse-rootfs-path-failed", err, lager.Data{
			"RootFSPath": spec.RootFSPath,
		})

		return "", nil, err
	}

	rootFSSpec := rootfs_provider.Spec{
		RootFS:     rootFSURL,
		Namespaced: resources.RootUID != 0,
		QuotaSize:  int64(spec.Limits.Disk.ByteHard),
		QuotaScope: spec.Limits.Disk.Scope,
	}

	pLog.Debug("provide-rootfs-starting")
	rootFSPath, rootFSEnvVars, err := p.rootFSProvider.Create(pLog, id, rootFSSpec)
	if err != nil {
		pLog.Error("provide-rootfs-failed", err)

		return "", nil, err
	}
	pLog.Debug("provide-rootfs-ended")

	pLog.Debug("clean-rootfs-starting")
	if err := p.rootFSCleaner.Clean(pLog, rootFSPath); err != nil {
		return "", nil, err
	}
	pLog.Debug("clean-rootfs-ended")

	rootFSProcessEnv, err := process.NewEnv(rootFSEnvVars)
	if err != nil {
		pLog.Error("rootfs-env-malformed", err)

		return "", nil, err
	}

	return rootFSPath, rootFSProcessEnv, nil
}

func (p *LinuxResourcePool) setupContainerDirectories(spec garden.ContainerSpec, id string, resources *linux_backend.Resources, pLog lager.Logger) (string, process.Env, error) {
	rootFSPath, rootFSEnvVars, err := p.setupRootfs(spec, id, resources, pLog)
	if err != nil {
		return "", nil, err
	}

	pLog.Debug("setup-bridge-starting")
	if err := p.setupBridge(pLog, id, resources); err != nil {
		p.rootFSProvider.Destroy(pLog, id)
		return "", nil, err
	}
	pLog.Debug("setup-bridge-ended")

	return rootFSPath, rootFSEnvVars, nil
}

func (p *LinuxResourcePool) setupBridge(pLog lager.Logger, id string, resources *linux_backend.Resources) error {
	var err error
	if resources.Bridge, err = p.bridges.Reserve(resources.Network.Subnet, id); err != nil {
		pLog.Error("reserve-bridge-failed", err, lager.Data{
			"Id":     id,
			"Subnet": resources.Network.Subnet,
			"Bridge": resources.Bridge,
		})

		return err
	}

	if err = p.saveBridgeName(id, resources.Bridge); err != nil {
		pLog.Error("save-bridge-name-failed", err, lager.Data{
			"Id":     id,
			"Bridge": resources.Bridge,
		})

		return err
	}

	return nil
}

func (p *LinuxResourcePool) tryReleaseSystemResources(logger lager.Logger, id string) {
	err := p.releaseSystemResources(logger, id)
	if err != nil {
		logger.Error("failed-to-undo-failed-create", err)
	}
}

func (p *LinuxResourcePool) releaseSystemResources(logger lager.Logger, id string) error {
	pRunner := logging.Runner{
		CommandRunner: p.runner,
		Logger:        logger,
	}

	bridgeName, err := ioutil.ReadFile(path.Join(p.depotPath, id, "bridge-name"))
	if err == nil {
		if err := p.bridges.Release(string(bridgeName), id); err != nil {
			return fmt.Errorf("containerpool: release bridge %s: %v", bridgeName, err)
		}
	}

	rootFSProvider, err := ioutil.ReadFile(path.Join(p.depotPath, id, "rootfs-provider"))
	if err != nil {
		rootFSProvider = []byte("invalid-rootfs-provider")
	}

	if err = p.iptablesMgr.ContainerTeardown(id); err != nil {
		return err
	}

	destroy := exec.Command(path.Join(p.binPath, "destroy.sh"), path.Join(p.depotPath, id))
	err = pRunner.Run(destroy)
	if err != nil {
		return err
	}

	if shouldCleanRootfs(string(rootFSProvider)) {
		if err = p.rootFSProvider.Destroy(logger, id); err != nil {
			return err
		}
	}

	p.filterProvider.ProvideFilter(id).TearDown()
	return nil
}

func shouldCleanRootfs(rootFSProvider string) bool {
	// invalid-rootfs-provider indicates that this is probably a recent container that failed on create.
	// we should try to clean it up

	providers := []string{
		"docker-local-aufs",
		"docker-local-vfs",
		"docker-remote-aufs",
		"docker-remote-vfs",
		"docker-composite",
		"invalid-rootfs-provider",
	}

	for _, provider := range providers {
		if provider == rootFSProvider {
			return true
		}
	}

	return false
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

func parseNetworkSpec(spec string) (subnets.SubnetSelector, subnets.IPSelector, error) {
	var ipSelector subnets.IPSelector = subnets.DynamicIPSelector
	var subnetSelector subnets.SubnetSelector = subnets.DynamicSubnetSelector

	if spec != "" {
		specifiedIP, ipn, err := net.ParseCIDR(suffixIfNeeded(spec))
		if err != nil {
			return nil, nil, err
		}

		subnetSelector = subnets.StaticSubnetSelector{ipn}

		if !specifiedIP.Equal(subnets.NetworkIP(ipn)) {
			ipSelector = subnets.StaticIPSelector{specifiedIP}
		}
	}

	return subnetSelector, ipSelector, nil
}

func suffixIfNeeded(spec string) string {
	if !strings.Contains(spec, "/") {
		spec = spec + "/30"
	}

	return spec
}
