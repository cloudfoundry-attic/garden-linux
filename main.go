package main

import (
	"bytes"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"

	"github.com/blang/semver"
	"github.com/cloudfoundry/gunk/command_runner"
	"github.com/docker/docker/daemon/graphdriver"
	_ "github.com/docker/docker/daemon/graphdriver/btrfs"
	_ "github.com/docker/docker/daemon/graphdriver/vfs"
	"github.com/docker/docker/graph"
	_ "github.com/docker/docker/pkg/chrootarchive" // allow reexec of docker-applyLayer
	"github.com/docker/docker/registry"
	"github.com/pivotal-golang/lager"
	"github.com/pivotal-golang/localip"

	"github.com/cloudfoundry-incubator/cf-debug-server"
	"github.com/cloudfoundry-incubator/cf-lager"
	"github.com/cloudfoundry-incubator/garden-linux/container_repository"
	"github.com/cloudfoundry-incubator/garden-linux/debug"
	"github.com/cloudfoundry-incubator/garden-linux/linux_backend"
	"github.com/cloudfoundry-incubator/garden-linux/linux_container"
	"github.com/cloudfoundry-incubator/garden-linux/linux_container/bandwidth_manager"
	"github.com/cloudfoundry-incubator/garden-linux/linux_container/cgroups_manager"
	"github.com/cloudfoundry-incubator/garden-linux/linux_container/iptables_manager"
	"github.com/cloudfoundry-incubator/garden-linux/linux_container/quota_manager"
	"github.com/cloudfoundry-incubator/garden-linux/network"
	"github.com/cloudfoundry-incubator/garden-linux/network/bridgemgr"
	"github.com/cloudfoundry-incubator/garden-linux/network/devices"
	"github.com/cloudfoundry-incubator/garden-linux/network/iptables"
	"github.com/cloudfoundry-incubator/garden-linux/network/subnets"
	"github.com/cloudfoundry-incubator/garden-linux/pkg/vars"
	"github.com/cloudfoundry-incubator/garden-linux/port_pool"
	"github.com/cloudfoundry-incubator/garden-linux/process_tracker"
	"github.com/cloudfoundry-incubator/garden-linux/resource_pool"
	"github.com/cloudfoundry-incubator/garden-linux/shed/layercake"
	"github.com/cloudfoundry-incubator/garden-linux/shed/repository_fetcher"
	"github.com/cloudfoundry-incubator/garden-linux/shed/rootfs_provider"
	"github.com/cloudfoundry-incubator/garden-linux/sysconfig"
	"github.com/cloudfoundry-incubator/garden-linux/sysinfo"
	"github.com/cloudfoundry-incubator/garden/server"
	"github.com/cloudfoundry/dropsonde"
	"github.com/cloudfoundry/gunk/command_runner/linux_command_runner"
	"github.com/docker/docker/pkg/reexec"
)

const (
	DefaultNetworkPool      = "10.254.0.0/22"
	DefaultMTUSize          = 1500
	CurrentContainerVersion = "1.0.0"
)

var listenNetwork = flag.String(
	"listenNetwork",
	"unix",
	"how to listen on the address (unix, tcp, etc.)",
)

var listenAddr = flag.String(
	"listenAddr",
	"/tmp/garden.sock",
	"address to listen on",
)

var snapshotsPath = flag.String(
	"snapshots",
	"",
	"directory in which to store container state to persist through restarts",
)

var binPath = flag.String(
	"bin",
	"",
	"directory containing backend-specific scripts (i.e. ./create.sh)",
)

var depotPath = flag.String(
	"depot",
	"",
	"directory in which to store containers",
)

var rootFSPath = flag.String(
	"rootfs",
	"",
	"directory of the rootfs for the containers",
)

var enableGraphCleanup = flag.Bool(
	"enableGraphCleanup",
	false,
	"enables graph garbage collection",
)

var disableQuotas = flag.Bool(
	"disableQuotas",
	false,
	"disable disk quotas",
)

var containerGraceTime = flag.Duration(
	"containerGraceTime",
	0,
	"time after which to destroy idle containers",
)

var portPoolStart = flag.Uint(
	"portPoolStart",
	60000,
	"start of ephemeral port range used for mapped container ports",
)

var portPoolSize = flag.Uint(
	"portPoolSize",
	5000,
	"size of port pool used for mapped container ports",
)

var networkPool = flag.String("networkPool",
	DefaultNetworkPool,
	"Pool of dynamically allocated container subnets")

var denyNetworks = flag.String(
	"denyNetworks",
	"",
	"CIDR blocks representing IPs to blacklist",
)

var allowNetworks = flag.String(
	"allowNetworks",
	"",
	"CIDR blocks representing IPs to whitelist",
)

var graphRoot = flag.String(
	"graph",
	"/var/lib/garden-docker-graph",
	"docker image graph",
)

var dockerRegistry = flag.String(
	"registry",
	registry.IndexServerAddress(),
	"docker registry API endpoint",
)

var tag = flag.String(
	"tag",
	"",
	"server-wide identifier used for 'global' configuration, must be less than 3 character long",
)

var dropsondeOrigin = flag.String(
	"dropsondeOrigin",
	"garden-linux",
	"Origin identifier for dropsonde-emitted metrics.",
)

var dropsondeDestination = flag.String(
	"dropsondeDestination",
	"localhost:3457",
	"Destination for dropsonde-emitted metrics.",
)

var allowHostAccess = flag.Bool(
	"allowHostAccess",
	false,
	"allow network access to host",
)

var iptablesLogMethod = flag.String(
	"iptablesLogMethod",
	"kernel",
	"type of iptable logging to use, one of 'kernel' or 'nflog' (default: kernel)",
)

var mtu = flag.Int(
	"mtu",
	DefaultMTUSize,
	"MTU size for container network interfaces")

var externalIP = flag.String(
	"externalIP",
	"",
	"IP address to use to reach container's mapped ports")

var maxContainers = flag.Uint(
	"maxContainers",
	0,
	"Maximum number of containers that can be created")

func main() {
	if reexec.Init() {
		return
	}

	var insecureRegistries vars.StringList
	flag.Var(
		&insecureRegistries,
		"insecureDockerRegistry",
		"Docker registry to allow connecting to even if not secure. (Can be specified multiple times to allow insecure connection to multiple repositories)",
	)

	var persistentImages vars.StringList
	flag.Var(
		&persistentImages,
		"persistentImage",
		"Image which should never be garbage collected. (Can be specified multiple times)",
	)

	cf_debug_server.AddFlags(flag.CommandLine)
	cf_lager.AddFlags(flag.CommandLine)
	flag.Parse()

	runtime.GOMAXPROCS(runtime.NumCPU())

	logger, reconfigurableSink := cf_lager.New("garden-linux")
	if dbgAddr := cf_debug_server.DebugAddress(flag.CommandLine); dbgAddr != "" {
		debug.Run(dbgAddr, reconfigurableSink)
	}

	initializeDropsonde(logger)

	if *binPath == "" {
		missing("-bin")
	}

	if *depotPath == "" {
		missing("-depot")
	}

	if len(*tag) > 2 {
		println("-tag parameter must be less than 3 characters long")
		println()
		flag.Usage()
		return
	}

	_, dynamicRange, err := net.ParseCIDR(*networkPool)
	if err != nil {
		logger.Fatal("failed-to-parse-network-pool", err)
	}

	subnetPool, err := subnets.NewSubnets(dynamicRange)
	if err != nil {
		logger.Fatal("failed-to-create-subnet-pool", err)
	}

	// TODO: use /proc/sys/net/ipv4/ip_local_port_range by default (end + 1)
	portPool, err := port_pool.New(uint32(*portPoolStart), uint32(*portPoolSize))
	if err != nil {
		logger.Fatal("invalid pool range", err)
	}

	useKernelLogging := true
	switch *iptablesLogMethod {
	case "nflog":
		useKernelLogging = false
	case "kernel":
		/* noop */
	default:
		println("-iptablesLogMethod value not recognized")
		println()
		flag.Usage()
		return
	}

	config := sysconfig.NewConfig(*tag, *allowHostAccess)

	runner := sysconfig.NewRunner(config, linux_command_runner.New())

	if err := os.MkdirAll(*graphRoot, 0755); err != nil {
		logger.Fatal("failed-to-create-graph-directory", err)
	}

	dockerGraphDriver, err := graphdriver.New(*graphRoot, nil)
	if err != nil {
		logger.Fatal("failed-to-construct-graph-driver", err)
	}

	dockerGraph, err := graph.NewGraph(*graphRoot, dockerGraphDriver)
	if err != nil {
		logger.Fatal("failed-to-construct-graph", err)
	}

	graphMountPoint := mountPoint(logger, *graphRoot)

	var cake layercake.Cake = &layercake.Docker{
		Graph: dockerGraph,
	}

	if cake.DriverName() == "btrfs" {
		cake = &layercake.BtrfsCleaningCake{
			Cake:            cake,
			Runner:          runner,
			BtrfsMountPoint: graphMountPoint,
			RemoveAll:       os.RemoveAll,
			Logger:          logger.Session("btrfs-cleanup"),
		}
	}

	ovenCleanerCake := &layercake.OvenCleaner{
		Cake:               cake,
		Logger:             logger.Session("oven-cleaner"),
		EnableImageCleanup: *enableGraphCleanup,
	}

	lock := repository_fetcher.NewFetchLock()

	repoFetcher := &repository_fetcher.CompositeFetcher{
		LocalFetcher: &repository_fetcher.Local{
			Cake:              ovenCleanerCake,
			DefaultRootFSPath: *rootFSPath,
			IDProvider:        repository_fetcher.LayerIDProvider{},
		},
		RemoteFetchers: map[registry.APIVersion]repository_fetcher.VersionedFetcher{
			registry.APIVersion1: &repository_fetcher.RemoteV1Fetcher{
				Cake:      ovenCleanerCake,
				GraphLock: lock,
			},
			registry.APIVersion2: &repository_fetcher.RemoteV2Fetcher{
				Cake:      ovenCleanerCake,
				GraphLock: lock,
			},
		},
		RequestCreator: &repository_fetcher.RemoteFetchRequestCreator{
			RegistryProvider: repository_fetcher.NewRepositoryProvider(
				*dockerRegistry,
				insecureRegistries.List,
			),
			Pinger: repository_fetcher.EndpointPinger{},
			Logger: logger.Session("remote-fetch-request-creator"),
		},
	}

	maxId := sysinfo.Min(sysinfo.MustGetMaxValidUID(), sysinfo.MustGetMaxValidGID())
	mappingList := rootfs_provider.MappingList{
		{
			FromID: 0,
			ToID:   maxId,
			Size:   1,
		},
		{
			FromID: 1,
			ToID:   1,
			Size:   maxId - 1,
		},
	}

	rootFSNamespacer := &rootfs_provider.UidNamespacer{
		Logger: logger,
		Translator: rootfs_provider.NewUidTranslator(
			mappingList, // uid
			mappingList, // gid
		),
	}

	layerCreator := rootfs_provider.NewLayerCreator(
		ovenCleanerCake, rootfs_provider.SimpleVolumeCreator{}, rootFSNamespacer)

	cakeOrdinator := rootfs_provider.NewCakeOrdinator(
		ovenCleanerCake, repoFetcher, layerCreator, ovenCleanerCake, logger.Session("cake-ordinator"),
	)

	imageRetainer := &repository_fetcher.ImageRetainer{
		GraphRetainer:             cakeOrdinator,
		DirectoryRootfsIDProvider: repository_fetcher.LayerIDProvider{},
		DockerImageIDFetcher:      repoFetcher,

		NamespaceCacheKey: rootFSNamespacer.CacheKey(),
		Logger:            logger,
	}

	// spawn off in a go function to avoid blocking startup
	// worst case is if an image is immediately created and deleted faster than
	// we can retain it we'll garbage collect it when we shouldn't. This
	// is an OK trade-off for not having garden startup block on dockerhub.
	go imageRetainer.Retain(persistentImages.List)

	if *externalIP == "" {
		ip, err := localip.LocalIP()
		if err != nil {
			panic("couldn't determine local IP to use for -externalIP parameter. You can use the -externalIP flag to pass an external IP")
		}

		externalIP = &ip
	}

	parsedExternalIP := net.ParseIP(*externalIP)
	if parsedExternalIP == nil {
		panic(fmt.Sprintf("Value of -externalIP %s could not be converted to an IP", *externalIP))
	}

	var quotaManager linux_container.QuotaManager = quota_manager.DisabledQuotaManager{}
	if !*disableQuotas {
		quotaManager = &quota_manager.BtrfsQuotaManager{
			Runner:     runner,
			MountPoint: graphMountPoint,
		}
	}

	ipTablesMgr := createIPTablesManager(config, runner, logger)
	injector := &provider{
		useKernelLogging: useKernelLogging,
		chainPrefix:      config.IPTables.Filter.InstancePrefix,
		runner:           runner,
		log:              logger,
		portPool:         portPool,
		ipTablesMgr:      ipTablesMgr,
		sysconfig:        config,
		quotaManager:     quotaManager,
	}

	currentContainerVersion, err := semver.Make(CurrentContainerVersion)
	if err != nil {
		logger.Fatal("failed-to-parse-container-version", err)
	}

	pool := resource_pool.New(
		logger,
		*binPath,
		*depotPath,
		config,
		cakeOrdinator,
		mappingList,
		parsedExternalIP,
		*mtu,
		subnetPool,
		bridgemgr.New("w"+config.Tag+"b-", &devices.Bridge{}, &devices.Link{}),
		ipTablesMgr,
		injector,
		iptables.NewGlobalChain(config.IPTables.Filter.DefaultChain, runner, logger.Session("global-chain")),
		portPool,
		strings.Split(*denyNetworks, ","),
		strings.Split(*allowNetworks, ","),
		runner,
		quotaManager,
		currentContainerVersion,
	)

	systemInfo := sysinfo.NewProvider(*depotPath)

	backend := linux_backend.New(logger, pool, container_repository.New(), injector, systemInfo, layercake.GraphPath(*graphRoot), *snapshotsPath, int(*maxContainers))

	err = backend.Setup()
	if err != nil {
		logger.Fatal("failed-to-set-up-backend", err)
	}

	graceTime := *containerGraceTime

	gardenServer := server.New(*listenNetwork, *listenAddr, graceTime, backend, logger)

	err = gardenServer.Start()
	if err != nil {
		logger.Fatal("failed-to-start-server", err)
	}

	signals := make(chan os.Signal, 1)

	go func() {
		<-signals
		gardenServer.Stop()
		os.Exit(0)
	}()

	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	logger.Info("started", lager.Data{
		"network": *listenNetwork,
		"addr":    *listenAddr,
	})

	select {}
}

func mountPoint(logger lager.Logger, path string) string {
	dfOut := new(bytes.Buffer)

	df := exec.Command("df", path)
	df.Stdout = dfOut
	df.Stderr = os.Stderr

	err := df.Run()
	if err != nil {
		logger.Fatal("failed-to-get-mount-info", err)
	}

	dfOutputWords := strings.Split(string(dfOut.Bytes()), " ")

	return strings.Trim(dfOutputWords[len(dfOutputWords)-1], "\n")
}

func missing(flagName string) {
	println("missing " + flagName)
	println()
	flag.Usage()
	os.Exit(1)
}

func initializeDropsonde(logger lager.Logger) {
	err := dropsonde.Initialize(*dropsondeDestination, *dropsondeOrigin)
	if err != nil {
		logger.Error("failed to initialize dropsonde", err)
	}
}

func createIPTablesManager(sysconfig sysconfig.Config, runner command_runner.CommandRunner, log lager.Logger) linux_container.IPTablesManager {
	filterChain := iptables_manager.NewFilterChain(&sysconfig.IPTables.Filter, runner, log.Session("iptables-manager-filter"))
	natChain := iptables_manager.NewNATChain(&sysconfig.IPTables.NAT, runner, log.Session("iptables-manager-nat"))
	return iptables_manager.New().AddChain(filterChain).AddChain(natChain)
}

type provider struct {
	useKernelLogging bool
	chainPrefix      string
	runner           command_runner.CommandRunner
	log              lager.Logger
	portPool         *port_pool.PortPool
	ipTablesMgr      linux_container.IPTablesManager
	quotaManager     linux_container.QuotaManager
	sysconfig        sysconfig.Config
}

func (p *provider) ProvideFilter(containerId string) network.Filter {
	return network.NewFilter(iptables.NewLoggingChain(p.chainPrefix+containerId, p.useKernelLogging, p.runner, p.log.Session(containerId).Session("filter")))
}

func (p *provider) ProvideContainer(spec linux_backend.LinuxContainerSpec) linux_backend.Container {
	cgroupReader := &cgroups_manager.LinuxCgroupReader{
		Path: p.sysconfig.CgroupNodeFilePath,
	}

	cgroupsManager := cgroups_manager.New(p.sysconfig.CgroupPath, spec.ID, cgroupReader)

	oomWatcher := linux_container.NewOomNotifier(
		p.runner, spec.ContainerPath, cgroupsManager,
	)

	return linux_container.NewLinuxContainer(
		spec,
		p.portPool,
		p.runner,
		cgroupsManager,
		p.quotaManager,
		bandwidth_manager.New(spec.ContainerPath, spec.ID, p.runner),
		process_tracker.New(spec.ContainerPath, p.runner),
		p.ProvideFilter(spec.ID),
		p.ipTablesMgr,
		devices.Link{Name: p.sysconfig.NetworkInterfacePrefix + spec.ID + "-0"},
		oomWatcher,
		p.log.Session("container", lager.Data{"handle": spec.Handle}),
	)
}
