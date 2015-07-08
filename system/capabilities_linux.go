package system

import (
	"fmt"
	"runtime"

	"github.com/syndtr/gocapability/capability"
)

func init() {
	runtime.LockOSThread()
}

type ProcessCapabilities struct {
	Pid int
}

func (c ProcessCapabilities) Limit(extendedWhitelist bool) error {
	caps, err := capability.NewPid(c.Pid)
	if err != nil {
		return fmt.Errorf("system: getting capabilities: %s", err)
	}

	sets := capability.BOUNDING | capability.CAPS
	caps.Clear(sets)
	caps.Set(sets,
		capability.CAP_CHOWN,
		capability.CAP_DAC_OVERRIDE,
		capability.CAP_FSETID,
		capability.CAP_FOWNER,
		capability.CAP_MKNOD,
		capability.CAP_NET_RAW,
		capability.CAP_SETGID,
		capability.CAP_SETUID,
		capability.CAP_SETFCAP,
		capability.CAP_SETPCAP,
		capability.CAP_NET_BIND_SERVICE,
		capability.CAP_SYS_CHROOT,
		capability.CAP_KILL,
		capability.CAP_AUDIT_WRITE,
	)

	if extendedWhitelist {
		caps.Set(sets, capability.CAP_SYS_ADMIN)
	}

	err = caps.Apply(sets)
	if err != nil {
		return fmt.Errorf("system: applying capabilities: %s", err)
	}

	return nil
}
