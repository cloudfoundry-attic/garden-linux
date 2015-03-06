package old

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

	"github.com/cloudfoundry/gunk/command_runner"
	"github.com/cloudfoundry/gunk/localip"
	"github.com/docker/docker/daemon/graphdriver"
	_ "github.com/docker/docker/daemon/graphdriver/aufs"
	_ "github.com/docker/docker/daemon/graphdriver/vfs"
	"github.com/docker/docker/graph"
	"github.com/docker/docker/registry"
	"github.com/pivotal-golang/clock"
	"github.com/pivotal-golang/lager"

	"github.com/cloudfoundry-incubator/cf-debug-server"
	"github.com/cloudfoundry-incubator/cf-lager"
	"github.com/cloudfoundry-incubator/garden-linux/container_pool"
	"github.com/cloudfoundry-incubator/garden-linux/network"
	"github.com/cloudfoundry-incubator/garden-linux/network/cnet"
	"github.com/cloudfoundry-incubator/garden-linux/network/iptables"
	"github.com/cloudfoundry-incubator/garden-linux/network/subnets"
	"github.com/cloudfoundry-incubator/garden-linux/old/linux_backend"
	"github.com/cloudfoundry-incubator/garden-linux/old/linux_backend/container_pool/repository_fetcher"
	"github.com/cloudfoundry-incubator/garden-linux/old/linux_backend/container_pool/rootfs_provider"
	"github.com/cloudfoundry-incubator/garden-linux/old/linux_backend/port_pool"
	"github.com/cloudfoundry-incubator/garden-linux/old/linux_backend/quota_manager"
	"github.com/cloudfoundry-incubator/garden-linux/old/linux_backend/uid_pool"
	"github.com/cloudfoundry-incubator/garden-linux/old/sysconfig"
	"github.com/cloudfoundry-incubator/garden-linux/old/system_info"
	"github.com/cloudfoundry-incubator/garden/server"
	"github.com/cloudfoundry/dropsonde"
	"github.com/cloudfoundry/gunk/command_runner/linux_command_runner"
)

const (
	DefaultNetworkPool = "10.254.0.0/22"
	DefaultMTUSize     = 1500
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

var overlaysPath = flag.String(
	"overlays",
	"",
	"directory in which to store containers mount points",
)

var rootFSPath = flag.String(
	"rootfs",
	"",
	"directory of the rootfs for the containers",
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
	61001,
	"start of ephemeral port range used for mapped container ports",
)

var portPoolSize = flag.Uint(
	"portPoolSize",
	5000,
	"size of port pool used for mapped container ports",
)

var uidPoolStart = flag.Uint(
	"uidPoolStart",
	10000,
	"start of per-container user ids",
)

var uidPoolSize = flag.Uint(
	"uidPoolSize",
	256,
	"size of the uid pool",
)

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

var insecureRegistries = flag.String(
	"insecureDockerRegistryList",
	"",
	"comma-separated list of docker registries to allow connection to even if they are not secure",
)

var tag = &cnet.Tag // flag defined in cnet/init.go

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

func Main(builder cnet.Builder) {

	cf_debug_server.Run()

	runtime.GOMAXPROCS(runtime.NumCPU())

	logger := cf_lager.New("garden-linux")

	initializeDropsonde(logger)

	if *binPath == "" {
		missing("-bin")
	}

	if *depotPath == "" {
		missing("-depot")
	}

	if *overlaysPath == "" {
		missing("-overlays")
	}

	uidPool := uid_pool.New(uint32(*uidPoolStart), uint32(*uidPoolSize))

	_, dynamicRange, _ := net.ParseCIDR(cnet.DefaultNetworkPool)
	subnets, _ := subnets.NewSubnets(dynamicRange)
	subnetPool := &MySubnetPool{subnets}

	// TODO: use /proc/sys/net/ipv4/ip_local_port_range by default (end + 1)
	portPool := port_pool.New(uint32(*portPoolStart), uint32(*portPoolSize))

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

	quotaManager := quota_manager.New(runner, getMountPoint(logger, *depotPath), *binPath)

	if *disableQuotas {
		quotaManager.Disable()
	}

	if err := os.MkdirAll(*graphRoot, 0755); err != nil {
		logger.Fatal("failed-to-create-graph-directory", err)
	}

	graphDriver, err := graphdriver.New(*graphRoot, nil)
	if err != nil {
		logger.Fatal("failed-to-construct-graph-driver", err)
	}

	graph, err := graph.NewGraph(*graphRoot, graphDriver)
	if err != nil {
		logger.Fatal("failed-to-construct-graph", err)
	}

	repoFetcher := repository_fetcher.Retryable{
		repository_fetcher.New(
			repository_fetcher.NewRepositoryProvider(
				*dockerRegistry,
				strings.Split(*insecureRegistries, ","),
			),
			graph,
		),
	}

	dockerRootFSProvider, err := rootfs_provider.NewDocker(repoFetcher, graphDriver, rootfs_provider.SimpleVolumeCreator{}, clock.NewClock())
	if err != nil {
		logger.Fatal("failed-to-construct-docker-rootfs-provider", err)
	}

	rootFSProviders := map[string]rootfs_provider.RootFSProvider{
		"":       rootfs_provider.NewOverlay(*binPath, *overlaysPath, *rootFSPath, runner),
		"docker": dockerRootFSProvider,
	}

	filterProvider := &provider{
		useKernelLogging: useKernelLogging,
		chainPrefix:      config.IPTables.Filter.InstancePrefix,
		runner:           runner,
		log:              logger,
	}

	var parsedExternalIP net.IP
	if *externalIP == "" {
		parsedExternalIP, err = localip.LocalIP()
		if err != nil {
			panic("couldn't determine local IP to use for -externalIP parameter. You can use the -externalIP flag to pass an external IP")
		}
	} else {
		parsedExternalIP = net.ParseIP(*externalIP)
	}

	if externalIP == nil {
		panic(fmt.Sprintf("Value of -externalIP %s could not be converted to an IP", externalIP))
	}

	pool := container_pool.New(
		logger,
		*binPath,
		*depotPath,
		config,
		rootFSProviders,
		uidPool,
		*mtu,
		parsedExternalIP,
		subnetPool,
		builder,
		container_pool.NewCNPersistor(logger, builder),
		filterProvider,
		iptables.NewGlobalChain(config.IPTables.Filter.DefaultChain, runner, logger.Session("global-chain")),
		portPool,
		strings.Split(*denyNetworks, ","),
		strings.Split(*allowNetworks, ","),
		runner,
		quotaManager,
	)

	systemInfo := system_info.NewProvider(*depotPath)

	backend := linux_backend.New(logger, pool, systemInfo, *snapshotsPath)

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

func getMountPoint(logger lager.Logger, depotPath string) string {
	dfOut := new(bytes.Buffer)

	df := exec.Command("df", depotPath)
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
}

func initializeDropsonde(logger lager.Logger) {
	err := dropsonde.Initialize(*dropsondeDestination, *dropsondeOrigin)
	if err != nil {
		logger.Error("failed to initialize dropsonde: %v", err)
	}
}

type provider struct {
	useKernelLogging bool
	chainPrefix      string
	runner           command_runner.CommandRunner
	log              lager.Logger
}

func (p *provider) ProvideFilter(containerId string) network.Filter {
	return network.NewFilter(iptables.NewLoggingChain(p.chainPrefix+containerId, p.useKernelLogging, p.runner, p.log.Session(containerId).Session("filter")))
}

type MySubnetPool struct {
	c subnets.Subnets
}

func (c MySubnetPool) Acquire(spec string) (*linux_backend.Network, error) {
	var ipSelector subnets.IPSelector = subnets.DynamicIPSelector
	var subnetSelector subnets.SubnetSelector = subnets.DynamicSubnetSelector

	if spec != "" {
		specifiedIP, ipn, err := net.ParseCIDR(suffixIfNeeded(spec))
		if err != nil {
			return nil, err
		}

		subnetSelector = subnets.StaticSubnetSelector{ipn}

		if !specifiedIP.Equal(subnets.NetworkIP(ipn)) {
			ipSelector = subnets.StaticIPSelector{specifiedIP}
		}
	}

	subnet, containerIP, _, err := c.c.Allocate(subnetSelector, ipSelector)
	if err != nil {
		return nil, err
	}

	return &linux_backend.Network{IP: containerIP, Subnet: subnet}, nil
}

func (c MySubnetPool) Remove(n *linux_backend.Network) error {
	return c.c.Recover(n.Subnet, n.IP)
}

func (c MySubnetPool) Release(n *linux_backend.Network) error {
	_, err := c.c.Release(n.Subnet, n.IP)
	return err
}

func suffixIfNeeded(spec string) string {
	if !strings.Contains(spec, "/") {
		spec = spec + "/30"
	}

	return spec
}
