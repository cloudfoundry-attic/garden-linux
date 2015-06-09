package container_daemon

import (
	"fmt"
	"os"
	"os/exec"
	osuser "os/user"
	"strings"
	"syscall"

	"github.com/cloudfoundry-incubator/garden"
)

//go:generate counterfeiter -o fake_rlimits_env_encoder/fake_rlimits_env_encoder.go . RlimitsEnvEncoder
type RlimitsEnvEncoder interface {
	EncodeLimits(garden.ResourceLimits) string
}

//go:generate counterfeiter -o fake_user/fake_user.go . User
type User interface {
	Lookup(name string) (*osuser.User, error)
}

type ProcessSpecPreparer struct {
	Users           User
	ProcStarterPath string
	Rlimits         RlimitsEnvEncoder
}

const RLimitsTag = "ENCODEDRLIMITS"

func (p *ProcessSpecPreparer) PrepareCmd(spec garden.ProcessSpec) (*exec.Cmd, error) {
	rlimitsEnv := p.Rlimits.EncodeLimits(spec.Limits)
	rlimitArg := fmt.Sprintf("%s=%s", RLimitsTag, rlimitsEnv)
	args := append([]string{rlimitArg}, spec.Path)
	args = append(args, spec.Args...)
	cmd := exec.Command(p.ProcStarterPath, args...)

	cmd.Env = spec.Env

	var uid, gid uint32
	if user, err := p.Users.Lookup(spec.User); err == nil && user != nil {
		fmt.Sscanf(user.Uid, "%d", &uid) // todo(jz): handle errors
		fmt.Sscanf(user.Gid, "%d", &gid)
		cmd.Env = append(cmd.Env, "USER="+spec.User)
		cmd.Env = append(cmd.Env, "HOME="+user.HomeDir)

		if spec.Dir != "" {
			cmd.Dir = spec.Dir
		} else {
			cmd.Dir = user.HomeDir
		}
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
