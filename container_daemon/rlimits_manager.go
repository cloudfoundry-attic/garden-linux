package container_daemon

import (
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"
	"syscall"

	"code.cloudfoundry.org/garden"
)

const (
	RLIMIT_CPU        = syscall.RLIMIT_CPU    // 0
	RLIMIT_FSIZE      = syscall.RLIMIT_FSIZE  // 1
	RLIMIT_DATA       = syscall.RLIMIT_DATA   // 2
	RLIMIT_STACK      = syscall.RLIMIT_STACK  // 3
	RLIMIT_CORE       = syscall.RLIMIT_CORE   // 4
	RLIMIT_RSS        = 5                     // 5
	RLIMIT_NPROC      = 6                     // 6
	RLIMIT_NOFILE     = syscall.RLIMIT_NOFILE // 7
	RLIMIT_MEMLOCK    = 8                     // 8
	RLIMIT_AS         = syscall.RLIMIT_AS     // 9
	RLIMIT_LOCKS      = 10                    // 10
	RLIMIT_SIGPENDING = 11                    // 11
	RLIMIT_MSGQUEUE   = 12                    // 12
	RLIMIT_NICE       = 13                    // 13
	RLIMIT_RTPRIO     = 14                    // 14
	RLIMIT_INFINITY   = ^uint64(0)
)

type rlimitEntry struct {
	Id  int
	Max uint64
}

type RlimitsManager struct{}

func (mgr *RlimitsManager) Init() error {
	maxNoFile, err := mgr.MaxNoFile()
	if err != nil {
		return err
	}

	rLimitsMap := map[string]*rlimitEntry{
		"cpu":        &rlimitEntry{Id: RLIMIT_CPU, Max: RLIMIT_INFINITY},
		"fsize":      &rlimitEntry{Id: RLIMIT_FSIZE, Max: RLIMIT_INFINITY},
		"data":       &rlimitEntry{Id: RLIMIT_DATA, Max: RLIMIT_INFINITY},
		"stack":      &rlimitEntry{Id: RLIMIT_STACK, Max: RLIMIT_INFINITY},
		"core":       &rlimitEntry{Id: RLIMIT_CORE, Max: RLIMIT_INFINITY},
		"rss":        &rlimitEntry{Id: RLIMIT_RSS, Max: RLIMIT_INFINITY},
		"nproc":      &rlimitEntry{Id: RLIMIT_NPROC, Max: RLIMIT_INFINITY},
		"nofile":     &rlimitEntry{Id: RLIMIT_NOFILE, Max: maxNoFile},
		"memlock":    &rlimitEntry{Id: RLIMIT_MEMLOCK, Max: RLIMIT_INFINITY},
		"as":         &rlimitEntry{Id: RLIMIT_AS, Max: RLIMIT_INFINITY},
		"locks":      &rlimitEntry{Id: RLIMIT_LOCKS, Max: RLIMIT_INFINITY},
		"sigpending": &rlimitEntry{Id: RLIMIT_SIGPENDING, Max: RLIMIT_INFINITY},
		"msgqueue":   &rlimitEntry{Id: RLIMIT_MSGQUEUE, Max: RLIMIT_INFINITY},
		"nice":       &rlimitEntry{Id: RLIMIT_NICE, Max: RLIMIT_INFINITY},
		"rtprio":     &rlimitEntry{Id: RLIMIT_RTPRIO, Max: RLIMIT_INFINITY},
	}

	for label, entry := range rLimitsMap {
		if err := setHardRLimit(label, entry.Id, entry.Max); err != nil {
			return err
		}
	}

	return nil
}

func (*RlimitsManager) Apply(rlimits garden.ResourceLimits) error {
	rlimitFlags := getSystemRlimitFlags(rlimits)
	if len(rlimitFlags) == 0 {
		return nil
	}

	for whichRLimit, value := range rlimitFlags {
		if err := syscall.Setrlimit(whichRLimit, &syscall.Rlimit{value, value}); err != nil {
			return fmt.Errorf("container_daemon: setting rlimit: %s", err)
		}
	}

	return nil
}

func (*RlimitsManager) EncodeLimits(rlimits garden.ResourceLimits) string {
	var limits []string

	if rlimits.As != nil {
		limits = append(limits, fmt.Sprintf("RLIMIT_AS=%d", *rlimits.As))
	}
	if rlimits.Core != nil {
		limits = append(limits, fmt.Sprintf("RLIMIT_CORE=%d", *rlimits.Core))
	}
	if rlimits.Cpu != nil {
		limits = append(limits, fmt.Sprintf("RLIMIT_CPU=%d", *rlimits.Cpu))
	}
	if rlimits.Data != nil {
		limits = append(limits, fmt.Sprintf("RLIMIT_DATA=%d", *rlimits.Data))
	}
	if rlimits.Fsize != nil {
		limits = append(limits, fmt.Sprintf("RLIMIT_FSIZE=%d", *rlimits.Fsize))
	}
	if rlimits.Locks != nil {
		limits = append(limits, fmt.Sprintf("RLIMIT_LOCKS=%d", *rlimits.Locks))
	}
	if rlimits.Memlock != nil {
		limits = append(limits, fmt.Sprintf("RLIMIT_MEMLOCK=%d", *rlimits.Memlock))
	}
	if rlimits.Msgqueue != nil {
		limits = append(limits, fmt.Sprintf("RLIMIT_MSGQUEUE=%d", *rlimits.Msgqueue))
	}
	if rlimits.Nice != nil {
		limits = append(limits, fmt.Sprintf("RLIMIT_NICE=%d", *rlimits.Nice))
	}
	if rlimits.Nofile != nil {
		limits = append(limits, fmt.Sprintf("RLIMIT_NOFILE=%d", *rlimits.Nofile))
	}
	if rlimits.Nproc != nil {
		limits = append(limits, fmt.Sprintf("RLIMIT_NPROC=%d", *rlimits.Nproc))
	}
	if rlimits.Rss != nil {
		limits = append(limits, fmt.Sprintf("RLIMIT_RSS=%d", *rlimits.Rss))
	}
	if rlimits.Rtprio != nil {
		limits = append(limits, fmt.Sprintf("RLIMIT_RTPRIO=%d", *rlimits.Rtprio))
	}
	if rlimits.Sigpending != nil {
		limits = append(limits, fmt.Sprintf("RLIMIT_SIGPENDING=%d", *rlimits.Sigpending))
	}
	if rlimits.Stack != nil {
		limits = append(limits, fmt.Sprintf("RLIMIT_STACK=%d", *rlimits.Stack))
	}

	return strings.Join(limits, ",")
}

func (*RlimitsManager) DecodeLimits(encodedLimits string) garden.ResourceLimits {
	limits := decode(encodedLimits)

	return garden.ResourceLimits{
		As:         limits["RLIMIT_AS"],
		Core:       limits["RLIMIT_CORE"],
		Cpu:        limits["RLIMIT_CPU"],
		Data:       limits["RLIMIT_DATA"],
		Fsize:      limits["RLIMIT_FSIZE"],
		Locks:      limits["RLIMIT_LOCKS"],
		Memlock:    limits["RLIMIT_MEMLOCK"],
		Msgqueue:   limits["RLIMIT_MSGQUEUE"],
		Nice:       limits["RLIMIT_NICE"],
		Nofile:     limits["RLIMIT_NOFILE"],
		Nproc:      limits["RLIMIT_NPROC"],
		Rss:        limits["RLIMIT_RSS"],
		Rtprio:     limits["RLIMIT_RTPRIO"],
		Sigpending: limits["RLIMIT_SIGPENDING"],
		Stack:      limits["RLIMIT_STACK"],
	}
}

func decode(encodedLimits string) map[string]*uint64 {
	env := make(map[string]*uint64)

	limits := strings.Split(encodedLimits, ",")

	for _, str := range limits {
		tokens := strings.SplitN(str, "=", 2)

		if len(tokens) == 2 {
			var val uint64
			n, err := fmt.Sscanf(tokens[1], "%d", &val)
			if err == nil && n == 1 {
				env[tokens[0]] = &val
			}
		}
	}

	return env
}

func (mgr *RlimitsManager) MaxNoFile() (uint64, error) {
	contents, err := ioutil.ReadFile("/proc/sys/fs/nr_open")
	if err != nil {
		return 0, fmt.Errorf("container_daemon: failed to read /proc/sys/fs/nr_open: %s", err)
	}

	contentStr := strings.TrimSpace(string(contents))
	maxFiles, err := strconv.ParseUint(contentStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("container_daemon: failed to convert contents of /proc/sys/fs/nr_open: %s", err)
	}

	return maxFiles, nil
}

func setHardRLimit(label string, rLimitId int, rLimitMax uint64) error {
	var rlimit syscall.Rlimit

	if err := syscall.Getrlimit(rLimitId, &rlimit); err != nil {
		return fmt.Errorf("container_daemon: getting system limit %s: %s", label, err)
	}

	rlimit.Max = rLimitMax
	if err := syscall.Setrlimit(rLimitId, &rlimit); err != nil {
		return fmt.Errorf("container_daemon: setting hard system limit %s: %s", label, err)
	}

	return nil
}

func getSystemRlimitFlags(rlimits garden.ResourceLimits) map[int]uint64 {
	m := make(map[int]uint64)

	if rlimits.Cpu != nil {
		m[RLIMIT_CPU] = *rlimits.Cpu
	}

	if rlimits.Fsize != nil {
		m[RLIMIT_FSIZE] = *rlimits.Fsize
	}

	if rlimits.Data != nil {
		m[RLIMIT_DATA] = *rlimits.Data
	}

	if rlimits.Stack != nil {
		m[RLIMIT_STACK] = *rlimits.Stack
	}

	if rlimits.Core != nil {
		m[RLIMIT_CORE] = *rlimits.Core
	}

	if rlimits.Rss != nil {
		m[RLIMIT_RSS] = *rlimits.Rss
	}

	if rlimits.Nproc != nil {
		m[RLIMIT_NPROC] = *rlimits.Nproc
	}

	if rlimits.Nofile != nil {
		m[RLIMIT_NOFILE] = *rlimits.Nofile
	}

	if rlimits.Memlock != nil {
		m[RLIMIT_MEMLOCK] = *rlimits.Memlock
	}

	if rlimits.As != nil {
		m[RLIMIT_AS] = *rlimits.As
	}

	if rlimits.Locks != nil {
		m[RLIMIT_LOCKS] = *rlimits.Locks
	}

	if rlimits.Sigpending != nil {
		m[RLIMIT_SIGPENDING] = *rlimits.Sigpending
	}

	if rlimits.Msgqueue != nil {
		m[RLIMIT_MSGQUEUE] = *rlimits.Msgqueue
	}

	if rlimits.Nice != nil {
		m[RLIMIT_NICE] = *rlimits.Nice
	}

	if rlimits.Rtprio != nil {
		m[RLIMIT_RTPRIO] = *rlimits.Rtprio
	}

	return m
}
