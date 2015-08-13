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
	cgroupsManager CgroupsManager

	doneWatching chan bool
}

func NewOomNotifier(runner command_runner.CommandRunner,
	containerPath string,
	cgroupsManager CgroupsManager) *OomNotifier {
	return &OomNotifier{
		mutex:          sync.RWMutex{},
		runner:         runner,
		containerPath:  containerPath,
		cgroupsManager: cgroupsManager,
	}
}

func (o *OomNotifier) Watch(onOom func()) error {
	o.mutex.Lock()
	defer o.mutex.Unlock()

	if o.cmd != nil {
		return nil
	}

	o.doneWatching = make(chan bool)
	go func() {
		if <-o.doneWatching {
			onOom()

			close(o.doneWatching)
		}
	}()

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

	go o.watch()

	return nil
}

func (o *OomNotifier) Unwatch() {
	o.mutex.RLock()
	defer o.mutex.RUnlock()

	if o.cmd != nil {
		o.runner.Kill(o.cmd)
	}
}

func (o *OomNotifier) watch() {
	o.mutex.RLock()
	cmd := o.cmd
	o.mutex.RUnlock()

	err := o.runner.Wait(cmd)
	o.mutex.Lock()
	o.cmd = nil
	o.mutex.Unlock()

	if err != nil {
		close(o.doneWatching)
	} else {
		o.doneWatching <- true
	}
}
