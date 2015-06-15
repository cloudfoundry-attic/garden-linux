package container_daemon

import (
	"fmt"
	"os/exec"
	osuser "os/user"
	"syscall"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry-incubator/garden-linux/process"
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

	env, err := process.NewEnv(spec.Env)
	if err != nil {
		return nil, fmt.Errorf("container_daemon: invalid environment %v: %s", spec.Env, err)
	}

	var uid, gid uint32
	if user, err := p.Users.Lookup(spec.User); err == nil && user != nil {
		fmt.Sscanf(user.Uid, "%d", &uid) // todo(jz): handle errors
		fmt.Sscanf(user.Gid, "%d", &gid)
		env["USER"] = spec.User
		_, hasHome := env["HOME"]
		if !hasHome {
			env["HOME"] = user.HomeDir
		}

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

	_, hasPath := env["PATH"]

	if !hasPath {
		if uid == 0 {
			env["PATH"] = DefaultRootPATH
		} else {
			env["PATH"] = DefaultUserPath
		}
	}

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{
			Uid: uid,
			Gid: gid,
		},
	}

	cmd.Env = env.Array()
	return cmd, nil
}
