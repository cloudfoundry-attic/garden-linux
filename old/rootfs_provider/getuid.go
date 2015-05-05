// +build !linux

package rootfs_provider

import "os"

func getuidgid(info os.FileInfo) (int, int, error) {
	panic("not supported")
}
