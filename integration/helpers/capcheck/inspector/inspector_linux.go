package inspector

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"syscall"

	"path/filepath"

	"github.com/syndtr/gocapability/capability"
)

func ProbeCAP_SYS_ADMIN() error {
	dirName, err := ioutil.TempDir("", "capability-utility")
	if err != nil {
		printMsg("CAP_SYS_ADMIN", "Failed to create a directory: %s", err)
		return err
	}
	defer os.RemoveAll(dirName)

	if err := syscall.Mount(dirName, dirName, "", uintptr(syscall.MS_BIND), ""); err != nil {
		printMsg("CAP_SYS_ADMIN", "Failed to create a bind mount: %s", err)
		return err
	} else {
		syscall.Unmount(dirName, 0)
		printMsg("CAP_SYS_ADMIN", "Create bind mount succeeded")
	}

	return nil
}

func ProbeCAP_NET_BIND_SERVICE() error {
	if ln, err := net.Listen("tcp", ":21"); err != nil {
		printMsg("CAP_NET_BIND_SERVICE", "Failed to create listener: %s", err)
		return err
	} else {
		ln.Close()
		printMsg("CAP_NET_BIND_SERVICE", "Create listener succeeded")
	}
	return nil
}

func ProbeCAP_MKNOD() error {
	dirName, err := ioutil.TempDir("", "CAP_MKNOD")
	if err != nil {
		printMsg("CAP_MKNOD", "Failed to create a directory: %s", err)
	}
	defer os.RemoveAll(dirName)

	if out, err := exec.Command("mknod", filepath.Join(dirName, "node"), "b", "0777", "200").CombinedOutput(); err != nil {
		printMsg("CAP_MKNOD", "Failed to make a node: %s, %s", err, string(out))
		return err
	} else {
		os.RemoveAll(dirName)
		printMsg("CAP_MKNOD", "Make node succeeded")
	}
	return nil
}

func PrintCaps() {
	PrintCap("CAP_DAC_OVERRIDE    ", capability.CAP_DAC_OVERRIDE)
	PrintCap("CAP_FSETID          ", capability.CAP_FSETID)
	PrintCap("CAP_FOWNER          ", capability.CAP_FOWNER)
	PrintCap("CAP_MKNOD           ", capability.CAP_MKNOD)
	PrintCap("CAP_NET_RAW         ", capability.CAP_NET_RAW)
	PrintCap("CAP_SETGID          ", capability.CAP_SETGID)
	PrintCap("CAP_SETUID          ", capability.CAP_SETUID)
	PrintCap("CAP_CHOWN           ", capability.CAP_CHOWN)
	PrintCap("CAP_SETFCAP         ", capability.CAP_SETFCAP)
	PrintCap("CAP_SETPCAP         ", capability.CAP_SETPCAP)
	PrintCap("CAP_NET_BIND_SERVICE", capability.CAP_NET_BIND_SERVICE)
	PrintCap("CAP_SYS_CHROOT      ", capability.CAP_SYS_CHROOT)
	PrintCap("CAP_KILL            ", capability.CAP_KILL)
	PrintCap("CAP_AUDIT_WRITE     ", capability.CAP_AUDIT_WRITE)
}

func PrintCap(capName string, cap capability.Cap) {
	caps, err := capability.NewPid(0)
	if err != nil {
		panic(err)
	}

	b := caps.Get(capability.BOUNDING, cap)
	p := caps.Get(capability.PERMITTED, cap)
	e := caps.Get(capability.EFFECTIVE, cap)
	i := caps.Get(capability.INHERITABLE, cap)

	fmt.Printf("%s bounding=%t, permitted=%t, effective=%t, inheritable=%t\n", capName, b, p, e, i)
}

func target(nonRootUid, nonRootGid int) (int, int) {
	if os.Getuid() == 0 {
		return nonRootUid, nonRootGid
	} else {
		return 0, 0
	}
}

func printMsg(tag, msg string, args ...interface{}) {
	text := fmt.Sprintf(msg, args...)
	fmt.Printf("%s: %s\n", tag, text)
}
