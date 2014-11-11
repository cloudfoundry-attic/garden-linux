package net_fence_test

import (
	"runtime"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestNetFence(t *testing.T) {
	RegisterFailHandler(Fail)
	BeforeSuite(optimiseScheduling)
	AfterSuite(resetScheduling)
	RunSpecs(t, "net-fence Suite")
}

var prevMaxProcs int

// Uses all CPUs for scheduling goroutines. The default in Go 1.3 is to use only one CPU.
func optimiseScheduling() {
	cpus := runtime.NumCPU()
	prevMaxProcs = runtime.GOMAXPROCS(cpus)
}

func resetScheduling() {
	runtime.GOMAXPROCS(prevMaxProcs)
}
