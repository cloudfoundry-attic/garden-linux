package runner

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/cloudfoundry-incubator/garden/client"
	"github.com/cloudfoundry-incubator/garden/client/connection"
	"github.com/onsi/ginkgo"
	"github.com/pivotal-golang/lager"
	"github.com/pivotal-golang/lager/lagertest"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/ginkgomon"
)

type Runner struct {
	Command *exec.Cmd

	network string
	addr    string

	bin  string
	argv []string

	binPath    string
	rootFSPath string

	tmpdir    string
	graphRoot string
	graphPath string

	btrfs bool
}

func New(network, addr string, bin, binPath, rootFSPath, graphRoot string, btrfs bool, argv ...string) *Runner {
	tmpDir := filepath.Join(
		os.TempDir(),
		fmt.Sprintf("test-garden-%d", ginkgo.GinkgoParallelNode()),
	)

	if graphRoot == "" {
		graphRoot = filepath.Join(tmpDir, "graph")
	}
	graphPath := filepath.Join(graphRoot, fmt.Sprintf("node-%d", ginkgo.GinkgoParallelNode()))

	return &Runner{
		network: network,
		addr:    addr,

		bin:  bin,
		argv: argv,

		binPath:    binPath,
		rootFSPath: rootFSPath,
		graphRoot:  graphRoot,
		graphPath:  graphPath,
		tmpdir:     tmpDir,

		btrfs: btrfs,
	}
}

func (r *Runner) Run(signals <-chan os.Signal, ready chan<- struct{}) error {
	logger := lagertest.NewTestLogger("garden-runner")

	if err := os.MkdirAll(r.tmpdir, 0755); err != nil {
		return err
	}

	depotPath := filepath.Join(r.tmpdir, "containers")
	snapshotsPath := filepath.Join(r.tmpdir, "snapshots")

	if err := os.MkdirAll(depotPath, 0755); err != nil {
		return err
	}

	if err := os.MkdirAll(snapshotsPath, 0755); err != nil {
		return err
	}

	//MustMountTmpfs(r.graphPath)

	var appendDefaultFlag = func(ar []string, key, value string) []string {
		for _, a := range r.argv {
			if a == key {
				return ar
			}
		}

		if value != "" {
			return append(ar, key, value)
		} else {
			return append(ar, key)
		}
	}

	gardenArgs := make([]string, len(r.argv))
	copy(gardenArgs, r.argv)

	gardenArgs = appendDefaultFlag(gardenArgs, "--listenNetwork", r.network)
	gardenArgs = appendDefaultFlag(gardenArgs, "--listenAddr", r.addr)
	gardenArgs = appendDefaultFlag(gardenArgs, "--bin", r.binPath)
	if r.rootFSPath != "" { //rootfs is an optional parameter
		gardenArgs = appendDefaultFlag(gardenArgs, "--rootfs", r.rootFSPath)
	}
	gardenArgs = appendDefaultFlag(gardenArgs, "--depot", depotPath)
	gardenArgs = appendDefaultFlag(gardenArgs, "--snapshots", snapshotsPath)
	gardenArgs = appendDefaultFlag(gardenArgs, "--graph", r.graphPath)
	gardenArgs = appendDefaultFlag(gardenArgs, "--logLevel", "debug")
	gardenArgs = appendDefaultFlag(gardenArgs, "--networkPool", fmt.Sprintf("10.250.%d.0/24", ginkgo.GinkgoParallelNode()))
	gardenArgs = appendDefaultFlag(gardenArgs, "--portPoolStart", strconv.Itoa(51000+(1000*ginkgo.GinkgoParallelNode())))
	gardenArgs = appendDefaultFlag(gardenArgs, "--portPoolSize", "1000")
	gardenArgs = appendDefaultFlag(gardenArgs, "--tag", strconv.Itoa(ginkgo.GinkgoParallelNode()))

	if !r.btrfs {
		gardenArgs = appendDefaultFlag(gardenArgs, "--disableQuotas", "")
	}

	gardenArgs = appendDefaultFlag(gardenArgs, "--debugAddr", fmt.Sprintf(":808%d", ginkgo.GinkgoParallelNode()))

	var signal os.Signal

	r.Command = exec.Command(r.bin, gardenArgs...)

	process := ifrit.Invoke(&ginkgomon.Runner{
		Name:              "garden-linux",
		Command:           r.Command,
		AnsiColorCode:     "31m",
		StartCheck:        "garden-linux.started",
		StartCheckTimeout: 10 * time.Second,
		Cleanup: func() {
			if signal == syscall.SIGQUIT {
				logger.Info("cleanup-subvolumes")

				// remove contents of subvolumes before deleting the subvolume
				if err := os.RemoveAll(r.graphPath); err != nil {
					logger.Error("remove graph", err)
				}

				if r.btrfs {
					// need to remove subvolumes before cleaning graphpath
					subvolumesOutput, err := exec.Command("btrfs", "subvolume", "list", r.graphRoot).CombinedOutput()
					logger.Debug(fmt.Sprintf("listing-subvolumes: %s", string(subvolumesOutput)))
					if err != nil {
						logger.Fatal("listing-subvolumes-error", err)
					}
					for _, line := range strings.Split(string(subvolumesOutput), "\n") {
						fields := strings.Fields(line)
						if len(fields) < 1 {
							continue
						}
						subvolumeRelativePath := fields[len(fields)-1]
						subvolumeAbsolutePath := filepath.Join(r.graphRoot, subvolumeRelativePath)
						if strings.Contains(subvolumeAbsolutePath, r.graphPath) {
							if b, err := exec.Command("btrfs", "subvolume", "delete", subvolumeAbsolutePath).CombinedOutput(); err != nil {
								logger.Fatal(fmt.Sprintf("deleting-subvolume: %s", string(b)), err)
							}
						}
					}

					if err := os.RemoveAll(r.graphPath); err != nil {
						logger.Error("remove graph again", err)
					}
				}

				logger.Info("cleanup-tempdirs")
				//MustUnmountTmpfs(overlaysPath)
				if err := os.RemoveAll(r.tmpdir); err != nil {
					logger.Error("cleanup-tempdirs-failed", err, lager.Data{"tmpdir": r.tmpdir})
				} else {
					logger.Info("tempdirs-removed")
				}
			}
		},
	})

	close(ready)

	for {
		select {
		case signal = <-signals:
			// SIGQUIT means clean up the containers, the garden process (SIGTERM) and the temporary directories
			// SIGKILL, SIGTERM and SIGINT are passed through to the garden process
			if signal == syscall.SIGQUIT {
				logger.Info("received-signal SIGQUIT")
				if err := r.destroyContainers(); err != nil {
					logger.Error("destroy-containers-failed", err)
					return err
				}
				logger.Info("destroyed-containers")
				process.Signal(syscall.SIGTERM)
			} else {
				logger.Info("received-signal", lager.Data{"signal": signal})
				process.Signal(signal)
			}

		case waitErr := <-process.Wait():
			logger.Info("process-exited")
			return waitErr
		}
	}
}

func (r *Runner) TryDial() error {
	conn, dialErr := net.DialTimeout(r.network, r.addr, 100*time.Millisecond)

	if dialErr == nil {
		conn.Close()
		return nil
	}

	return dialErr
}

func (r *Runner) NewClient() client.Client {
	return client.New(connection.New(r.network, r.addr))
}

func (r *Runner) destroyContainers() error {
	client := r.NewClient()

	containers, err := client.Containers(nil)
	if err != nil {
		return err
	}

	for _, container := range containers {
		err := client.Destroy(container.Handle())
		if err != nil {
			return err
		}
	}

	return nil
}
