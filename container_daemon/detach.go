package container_daemon

import (
	"os"
	"syscall"
)

// detach detaches from the origin stdin/out/err and makes
// sure the CWD is no longer inside the original host rootfs
func Detach(redirectStdout, redirectStderr string) {
	os.Chdir("/")

	devNull := must(os.Open("/dev/null"))
	syscall.Dup2(int(devNull.Fd()), int(os.Stdin.Fd()))

	newStdout := must(os.OpenFile(redirectStdout, os.O_WRONLY|os.O_CREATE, 0700))
	syscall.Dup2(int(newStdout.Fd()), int(os.Stdout.Fd()))

	newStderr := must(os.OpenFile(redirectStderr, os.O_WRONLY|os.O_CREATE, 0700))
	syscall.Dup2(int(newStderr.Fd()), int(os.Stderr.Fd()))
}

func must(f *os.File, err error) *os.File {
	if err != nil {
		panic(err)
	}

	return f
}
