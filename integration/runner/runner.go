package runner

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/cloudfoundry/gunk/runner_support"
	"github.com/vito/cmdtest"
	"github.com/vito/gordon"
)

type Runner struct {
	Network string
	Addr    string

	DepotPath     string
	BinPath       string
	RootFSPath    string
	SnapshotsPath string

	wardenBin string
	wardenCmd *exec.Cmd

	tmpdir string
}

func New(wardenPath, binPath, rootFSPath, network, addr string) (*Runner, error) {
	runner := &Runner{
		Network:    network,
		Addr:       addr,
		BinPath:    binPath,
		RootFSPath: rootFSPath,

		wardenBin: wardenPath,
	}

	return runner, runner.Prepare()
}

func (r *Runner) Prepare() error {
	var err error

	r.tmpdir, err = ioutil.TempDir(os.TempDir(), "warden-linux-server")
	if err != nil {
		return err
	}

	r.DepotPath = filepath.Join(r.tmpdir, "containers")
	r.SnapshotsPath = filepath.Join(r.tmpdir, "snapshots")

	if err := os.Mkdir(r.DepotPath, 0755); err != nil {
		return err
	}

	if err := os.Mkdir(r.SnapshotsPath, 0755); err != nil {
		return err
	}

	return nil
}

func (r *Runner) Start(argv ...string) error {
	wardenArgs := argv
	wardenArgs = append(
		wardenArgs,
		"--listenNetwork", r.Network,
		"--listenAddr", r.Addr,
		"--bin", r.BinPath,
		"--depot", r.DepotPath,
		"--rootfs", r.RootFSPath,
		"--snapshots", r.SnapshotsPath,
		"--debug",
		"--disableQuotas",
	)

	warden := exec.Command(r.wardenBin, wardenArgs...)

	warden.Stdout = os.Stdout
	warden.Stderr = os.Stderr

	_, err := cmdtest.StartWrapped(
		warden,
		runner_support.TeeToGinkgoWriter,
		runner_support.TeeToGinkgoWriter,
	)
	if err != nil {
		return err
	}

	r.wardenCmd = warden

	return r.WaitForStart()
}

func (r *Runner) Stop() error {
	if r.wardenCmd == nil {
		return nil
	}

	err := r.wardenCmd.Process.Signal(os.Interrupt)
	if err != nil {
		return err
	}

	stopped := make(chan bool, 1)
	stop := make(chan bool, 1)

	go r.WaitForStop(stopped, stop)

	timeout := 10 * time.Second

	select {
	case <-stopped:
		r.wardenCmd = nil
		return nil
	case <-time.After(timeout):
		stop <- true
		return fmt.Errorf("warden did not shut down within %s", timeout)
	}
}

func (r *Runner) DestroyContainers() error {
	err := exec.Command(filepath.Join(r.BinPath, "clear.sh"), r.DepotPath).Run()
	if err != nil {
		return err
	}

	if err := os.RemoveAll(r.SnapshotsPath); err != nil {
		return err
	}

	return nil
}

func (r *Runner) TearDown() error {
	err := r.DestroyContainers()
	if err != nil {
		return err
	}

	return os.RemoveAll(r.tmpdir)
}

func (r *Runner) NewClient() gordon.Client {
	return gordon.NewClient(&gordon.ConnectionInfo{
		Network: r.Network,
		Addr:    r.Addr,
	})
}

func (r *Runner) WaitForStart() error {
	timeout := 10 * time.Second
	timeoutTimer := time.NewTimer(timeout)

	for {
		conn, dialErr := net.Dial(r.Network, r.Addr)

		if dialErr == nil {
			conn.Close()
			return nil
		}

		select {
		case <-time.After(100 * time.Millisecond):
		case <-timeoutTimer.C:
			return fmt.Errorf("warden did not come up within %s", timeout)
		}
	}
}

func (r *Runner) WaitForStop(stopped chan<- bool, stop <-chan bool) {
	for {
		var err error

		conn, dialErr := net.Dial(r.Network, r.Addr)

		if dialErr == nil {
			conn.Close()
		}

		err = dialErr

		if err != nil {
			stopped <- true
			return
		}

		select {
		case <-stop:
			return
		case <-time.After(100 * time.Millisecond):
		}
	}
}
