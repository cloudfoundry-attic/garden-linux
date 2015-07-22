package linux_container

import (
	"fmt"
	"os/exec"
	"path"
	"sync"

	"github.com/cloudfoundry/gunk/command_runner"
)

type OomNotifier struct {
	mutex          sync.RWMutex
	cmd            *exec.Cmd
	runner         command_runner.CommandRunner
	containerPath  string
	stopCallback   func()
	cgroupsManager CgroupsManager
}

func NewOomNotifier(runner command_runner.CommandRunner,
	containerPath string,
	stopCallback func(),
	cgroupsManager CgroupsManager) *OomNotifier {
	return &OomNotifier{
		mutex:          sync.RWMutex{},
		runner:         runner,
		containerPath:  containerPath,
		stopCallback:   stopCallback,
		cgroupsManager: cgroupsManager,
	}
}

func (o *OomNotifier) Start() error {
	o.mutex.Lock()
	defer o.mutex.Unlock()

	if o.cmd != nil {
		return nil
	}

	oomPath := path.Join(o.containerPath, "bin", "oom")

	memorySubsystemPath, err := o.cgroupsManager.SubsystemPath("memory")
	if err != nil {
		return fmt.Errorf("linux_container: startOomNotifier: %s", err)
	}
	o.cmd = exec.Command(oomPath, memorySubsystemPath)

	err = o.runner.Start(o.cmd)
	if err != nil {
		return err
	}

	go o.watch(o.cmd)
	// o.watch(o.cmd)

	return nil
}

func (o *OomNotifier) Stop() {
	o.mutex.RLock()
	defer o.mutex.RUnlock()

	if o.cmd != nil {
		o.runner.Kill(o.cmd)
	}
}

func (o *OomNotifier) watch(cmd *exec.Cmd) {
	err := o.runner.Wait(cmd)
	if err == nil {
		// TODO: o.registerEvent("out of memory")
		o.stopCallback()
	}

	// TODO: handle case where oom notifier itself failed? kill container?
}
