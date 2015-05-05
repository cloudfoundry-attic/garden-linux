package rootfs_provider

import (
	"os"
	"syscall"
)

func getuidgid(info os.FileInfo) (int, int, error) {
	return int(info.Sys().(*syscall.Stat_t).Uid), int(info.Sys().(*syscall.Stat_t).Gid), nil
}
