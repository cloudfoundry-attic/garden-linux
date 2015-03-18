package linux_container

import (
	"fmt"
	"os/exec"
	"path"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry-incubator/garden-linux/linux_backend"
	"github.com/cloudfoundry-incubator/garden-linux/process"
	"github.com/pivotal-golang/lager"
)

func (c *LinuxContainer) Run(spec garden.ProcessSpec, processIO garden.ProcessIO) (garden.Process, error) {
	wshPath := path.Join(c.path, "bin", "wsh")
	sockPath := path.Join(c.path, "run", "wshd.sock")

	user := "vcap"
	if spec.Privileged {
		user = "root"
	}

	if spec.User != "" {
		user = spec.User
	}

	args := []string{"--socket", sockPath, "--user", user}

	specEnv, err := process.NewEnv(spec.Env)
	if err != nil {
		return nil, err
	}

	c.logger.Session("run").Debug("calculate-environment", lager.Data{
		"container-env": c.env,
		"run-env":       specEnv,
	})

	defaultEnv := langEnv().Merge(c.env)
	processEnv := defaultEnv.Merge(specEnv)

	for _, envVar := range processEnv.Array() {
		args = append(args, "--env", envVar)
	}

	if spec.Dir != "" {
		args = append(args, "--dir", spec.Dir)
	}

	processID := c.processIDPool.Next()

	pidfile := path.Join(c.path, "processes", fmt.Sprintf("%d.pid", processID))
	args = append(args, "--pidfile", pidfile)

	signaller := &linux_backend.NamespacedSignaller{
		Runner:        c.runner,
		ContainerPath: c.path,
		PidFilePath:   pidfile,
	}

	args = append(args, spec.Path)

	wsh := exec.Command(wshPath, append(args, spec.Args...)...)

	setRLimitsEnv(wsh, spec.Limits)

	return c.processTracker.Run(processID, wsh, processIO, spec.TTY, signaller)
}

func (c *LinuxContainer) Attach(processID uint32, processIO garden.ProcessIO) (garden.Process, error) {
	return c.processTracker.Attach(processID, processIO)
}

func setRLimitsEnv(cmd *exec.Cmd, rlimits garden.ResourceLimits) {
	if rlimits.As != nil {
		cmd.Env = append(cmd.Env, fmt.Sprintf("RLIMIT_AS=%d", *rlimits.As))
	}

	if rlimits.Core != nil {
		cmd.Env = append(cmd.Env, fmt.Sprintf("RLIMIT_CORE=%d", *rlimits.Core))
	}

	if rlimits.Cpu != nil {
		cmd.Env = append(cmd.Env, fmt.Sprintf("RLIMIT_CPU=%d", *rlimits.Cpu))
	}

	if rlimits.Data != nil {
		cmd.Env = append(cmd.Env, fmt.Sprintf("RLIMIT_DATA=%d", *rlimits.Data))
	}

	if rlimits.Fsize != nil {
		cmd.Env = append(cmd.Env, fmt.Sprintf("RLIMIT_FSIZE=%d", *rlimits.Fsize))
	}

	if rlimits.Locks != nil {
		cmd.Env = append(cmd.Env, fmt.Sprintf("RLIMIT_LOCKS=%d", *rlimits.Locks))
	}

	if rlimits.Memlock != nil {
		cmd.Env = append(cmd.Env, fmt.Sprintf("RLIMIT_MEMLOCK=%d", *rlimits.Memlock))
	}

	if rlimits.Msgqueue != nil {
		cmd.Env = append(cmd.Env, fmt.Sprintf("RLIMIT_MSGQUEUE=%d", *rlimits.Msgqueue))
	}

	if rlimits.Nice != nil {
		cmd.Env = append(cmd.Env, fmt.Sprintf("RLIMIT_NICE=%d", *rlimits.Nice))
	}

	if rlimits.Nofile != nil {
		cmd.Env = append(cmd.Env, fmt.Sprintf("RLIMIT_NOFILE=%d", *rlimits.Nofile))
	}

	if rlimits.Nproc != nil {
		cmd.Env = append(cmd.Env, fmt.Sprintf("RLIMIT_NPROC=%d", *rlimits.Nproc))
	}

	if rlimits.Rss != nil {
		cmd.Env = append(cmd.Env, fmt.Sprintf("RLIMIT_RSS=%d", *rlimits.Rss))
	}

	if rlimits.Rtprio != nil {
		cmd.Env = append(cmd.Env, fmt.Sprintf("RLIMIT_RTPRIO=%d", *rlimits.Rtprio))
	}

	if rlimits.Sigpending != nil {
		cmd.Env = append(cmd.Env, fmt.Sprintf("RLIMIT_SIGPENDING=%d", *rlimits.Sigpending))
	}

	if rlimits.Stack != nil {
		cmd.Env = append(cmd.Env, fmt.Sprintf("RLIMIT_STACK=%d", *rlimits.Stack))
	}
}
