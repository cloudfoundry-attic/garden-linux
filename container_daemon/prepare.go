package container_daemon

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry-incubator/garden-linux/containerizer/system"
)

//go:generate counterfeiter -o fake_iowirer/fake_iowirer.go . IOWirer
type IOWirer interface {
	Wire(cmd *exec.Cmd) ([]*os.File, error)
}

//go:generate counterfeiter -o fake_rlimits_env_encoder/fake_rlimits_env_encoder.go . RlimitsEnvEncoder
type RlimitsEnvEncoder interface {
	EncodeEnv(garden.ResourceLimits) []string
}

type ProcessSpecPreparer struct {
	Users           system.User
	ProcStarterPath string
	Rlimits         RlimitsEnvEncoder
}

func (p *ProcessSpecPreparer) PrepareCmd(spec garden.ProcessSpec) (*exec.Cmd, error) {
	args := append([]string{spec.Path}, spec.Args...)
	cmd := exec.Command(p.ProcStarterPath, args...)

	cmd.Env = spec.Env

	var uid, gid uint32
	if user, err := p.Users.Lookup(spec.User); err == nil && user != nil {
		fmt.Sscanf(user.Uid, "%d", &uid) // todo(jz): handle errors
		fmt.Sscanf(user.Gid, "%d", &gid)
		cmd.Env = append(cmd.Env, "USER="+spec.User)
		cmd.Env = append(cmd.Env, "HOME="+user.HomeDir)
	} else if err == nil {
		return nil, fmt.Errorf("container_daemon: failed to lookup user %s", spec.User)
	} else {
		return nil, fmt.Errorf("container_daemon: lookup user %s: %s", spec.User, err)
	}

	hasPath := false
	for _, env := range spec.Env {
		parts := strings.SplitN(env, "=", 2)
		if parts[0] == "PATH" {
			hasPath = true
			break
		}
	}
	if !hasPath {
		if uid == 0 {
			cmd.Env = append(cmd.Env, fmt.Sprintf("PATH=%s", DefaultRootPATH))
		} else {
			cmd.Env = append(cmd.Env, fmt.Sprintf("PATH=%s", DefaultUserPath))
		}
	}

	rlimitsEnv := p.Rlimits.EncodeEnv(spec.Limits)
	if len(rlimitsEnv) != 0 {
		cmd.Env = append(cmd.Env, rlimitsEnv...)
	}

	cmd.Dir = spec.Dir

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{
			Uid: uid,
			Gid: gid,
		},
	}

	return cmd, nil
}

func tryToReportErrorf(errWriter *os.File, format string, inserts ...interface{}) {
	message := fmt.Sprintf(format, inserts)
	errWriter.Write([]byte(message)) // Ignore error - nothing to do.
}
