package system

import (
	"fmt"

	"github.com/syndtr/gocapability/capability"
)

type ProcessCapabilities struct {
	Pid int
}

func (c ProcessCapabilities) Limit() error {
	caps, err := capability.NewPid(c.Pid)
	if err != nil {
		return fmt.Errorf("system: getting capabilities: %s", err)
	}

	caps.Clear(capability.BOUNDING)
	caps.Set(capability.BOUNDING,
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

	err = caps.Apply(capability.BOUNDING)
	if err != nil {
		return fmt.Errorf("system: applying capabilities: %s", err)
	}

	return nil
}
