// +build linux

package wshd_test

import (
	"syscall"
)

const (
	RLIMIT_AS         = syscall.RLIMIT_AS
	RLIMIT_CORE       = syscall.RLIMIT_CORE
	RLIMIT_CPU        = syscall.RLIMIT_CPU
	RLIMIT_DATA       = syscall.RLIMIT_DATA
	RLIMIT_FSIZE      = syscall.RLIMIT_FSIZE
	RLIMIT_LOCKS      = 10
	RLIMIT_MEMLOCK    = 8
	RLIMIT_MSGQUEUE   = 12
	RLIMIT_NICE       = 13
	RLIMIT_NOFILE     = syscall.RLIMIT_NOFILE
	RLIMIT_NPROC      = 6
	RLIMIT_RSS        = 5
	RLIMIT_RTPRIO     = 14
	RLIMIT_SIGPENDING = 11
	RLIMIT_STACK      = syscall.RLIMIT_STACK
)
