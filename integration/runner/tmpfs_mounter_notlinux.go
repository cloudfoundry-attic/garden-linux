// +build !linux

package runner

func MustMountTmpfs(destination string) {
	panic("not supported")
}
