package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/blang/semver"
	"github.com/cloudfoundry/gunk/command_runner"
	"github.com/docker/docker/daemon/graphdriver"
	"github.com/docker/docker/graph"
	_ "github.com/docker/docker/pkg/chrootarchive" // allow reexec of docker-applyLayer
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
	"github.com/cloudfoundry-incubator/garden-linux/sysconfig"
	"github.com/cloudfoundry-incubator/garden-linux/sysinfo"
	"github.com/cloudfoundry-incubator/garden-shed/distclient"
	quotaed_aufs "github.com/cloudfoundry-incubator/garden-shed/docker_drivers/aufs"
	"github.com/cloudfoundry-incubator/garden-shed/layercake"
	"github.com/cloudfoundry-incubator/garden-shed/repository_fetcher"
	"github.com/cloudfoundry-incubator/garden-shed/rootfs_provider"
	"github.com/cloudfoundry-incubator/garden/server"
	"github.com/cloudfoundry/dropsonde"
	"github.com/cloudfoundry/gunk/command_runner/linux_command_runner"
	_ "github.com/docker/docker/daemon/graphdriver/aufs"
	_ "github.com/docker/docker/pkg/chrootarchive" // allow reexec of docker-applyLayer
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
	"registry-1.docker.io",
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

	backingStoresPath := filepath.Join(*graphRoot, "backing_stores")
	if err := os.MkdirAll(backingStoresPath, 0660); err != nil {
		logger.Fatal("failed-to-mkdir-backing-stores", err)
	}

	quotaedGraphDriver := &quotaed_aufs.QuotaedDriver{
		GraphDriver: dockerGraphDriver,
		BackingStoreMgr: &quotaed_aufs.BackingStore{
			RootPath: backingStoresPath,
		},
		LoopMounter: &quotaed_aufs.Loop{},
		RootPath:    *graphRoot,
	}

	dockerGraph, err := graph.NewGraph(*graphRoot, quotaedGraphDriver)
	if err != nil {
		logger.Fatal("failed-to-construct-graph", err)
	}

	var cake layercake.Cake = &layercake.Docker{
		Graph:  dockerGraph,
		Driver: quotaedGraphDriver,
	}

	if cake.DriverName() == "aufs" {
		cake = &layercake.AufsCake{
			Cake:   cake,
			Runner: runner,
		}
	}

	repoFetcher := &repository_fetcher.CompositeFetcher{
		LocalFetcher: &repository_fetcher.Local{
			Cake:              cake,
			DefaultRootFSPath: *rootFSPath,
			IDProvider:        repository_fetcher.LayerIDProvider{},
		},
		RemoteFetcher: repository_fetcher.NewRemote(
			logger,
			"registry-1.docker.io",
			cake,
			distclient.NewDialer(insecureRegistries.List),
			repository_fetcher.VerifyFunc(repository_fetcher.Verify),
		),
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
		cake, rootfs_provider.SimpleVolumeCreator{}, rootFSNamespacer)

	cakeOrdinator := rootfs_provider.NewCakeOrdinator(
		cake, repoFetcher, layerCreator, rootfs_provider.CakeRetainer{Cake: cake}, logger.Session("cake-ordinator"),
	)

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
