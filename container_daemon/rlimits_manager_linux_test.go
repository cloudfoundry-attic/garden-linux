package container_daemon_test

import (
	"syscall"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry-incubator/garden-linux/container_daemon"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("RlimitsManager", func() {
	var rlimits garden.ResourceLimits
	var mgr *container_daemon.RlimitsManager
	var systemRlimits syscall.Rlimit
	var prevRlimit *syscall.Rlimit

	BeforeEach(func() {
		rlimits = garden.ResourceLimits{}
		mgr = new(container_daemon.RlimitsManager)
	})

	Describe("Init", func() {
		itSetRlimitValue := func(name string, id int, val uint64) {
			By("setting hard rlimit "+name, func() {
				Expect(syscall.Getrlimit(id, &systemRlimits)).To(Succeed())
				Expect(systemRlimits.Max).To(Equal(val))
			})
		}

		BeforeEach(func() {
			Expect(mgr.Init()).To(Succeed())
		})

		It("sets rlimits to their maximum", func() {
			itSetRlimitValue("cpu", container_daemon.RLIMIT_CPU, container_daemon.RLIMIT_INFINITY)
			itSetRlimitValue("fsize", container_daemon.RLIMIT_FSIZE, container_daemon.RLIMIT_INFINITY)
			itSetRlimitValue("data", container_daemon.RLIMIT_DATA, container_daemon.RLIMIT_INFINITY)
			itSetRlimitValue("stack", container_daemon.RLIMIT_STACK, container_daemon.RLIMIT_INFINITY)
			itSetRlimitValue("core", container_daemon.RLIMIT_CORE, container_daemon.RLIMIT_INFINITY)
			itSetRlimitValue("rss", container_daemon.RLIMIT_RSS, container_daemon.RLIMIT_INFINITY)
			itSetRlimitValue("nproc", container_daemon.RLIMIT_NPROC, container_daemon.RLIMIT_INFINITY)
			itSetRlimitValue("memlock", container_daemon.RLIMIT_MEMLOCK, container_daemon.RLIMIT_INFINITY)
			itSetRlimitValue("as", container_daemon.RLIMIT_AS, container_daemon.RLIMIT_INFINITY)
			itSetRlimitValue("locks", container_daemon.RLIMIT_LOCKS, container_daemon.RLIMIT_INFINITY)
			itSetRlimitValue("sigpending", container_daemon.RLIMIT_SIGPENDING, container_daemon.RLIMIT_INFINITY)
			itSetRlimitValue("msgqueue", container_daemon.RLIMIT_MSGQUEUE, container_daemon.RLIMIT_INFINITY)
			itSetRlimitValue("nice", container_daemon.RLIMIT_NICE, container_daemon.RLIMIT_INFINITY)
			itSetRlimitValue("rtprio", container_daemon.RLIMIT_RTPRIO, container_daemon.RLIMIT_INFINITY)
		})

		It("set appropriate max number of files", func() {
			noFiles, err := mgr.MaxNoFile()
			Expect(err).ToNot(HaveOccurred())
			Expect(syscall.Getrlimit(container_daemon.RLIMIT_NOFILE, &systemRlimits)).To(Succeed())
			Expect(systemRlimits.Max).To(Equal(noFiles))
		})
	})

	Describe("Encode / Decode roundtrip", func() {
		It("preserves the rlimit values", func() {
			var (
				valAs         uint64 = 1
				valCore       uint64 = 2
				valCpu        uint64 = 3
				valData       uint64 = 4
				valFsize      uint64 = 5
				valLocks      uint64 = 6
				valMemlock    uint64 = 7
				valMsgqueue   uint64 = 8
				valNice       uint64 = 9
				valNofile     uint64 = 10
				valNproc      uint64 = 11
				valRss        uint64 = 12
				valRtprio     uint64 = 13
				valSigpending uint64 = 14
				valStack      uint64 = 15
			)

			rlimits := garden.ResourceLimits{
				As:         &valAs,
				Core:       &valCore,
				Cpu:        &valCpu,
				Data:       &valData,
				Fsize:      &valFsize,
				Locks:      &valLocks,
				Memlock:    &valMemlock,
				Msgqueue:   &valMsgqueue,
				Nice:       &valNice,
				Nofile:     &valNofile,
				Nproc:      &valNproc,
				Rss:        &valRss,
				Rtprio:     &valRtprio,
				Sigpending: &valSigpending,
				Stack:      &valStack,
			}

			env := mgr.EncodeLimits(rlimits)

			newRlimits := mgr.DecodeLimits(env)
			Expect(*newRlimits.As).To(Equal(valAs))
			Expect(*newRlimits.Core).To(Equal(valCore))
			Expect(*newRlimits.Cpu).To(Equal(valCpu))
			Expect(*newRlimits.Data).To(Equal(valData))
			Expect(*newRlimits.Fsize).To(Equal(valFsize))
			Expect(*newRlimits.Locks).To(Equal(valLocks))
			Expect(*newRlimits.Memlock).To(Equal(valMemlock))
			Expect(*newRlimits.Msgqueue).To(Equal(valMsgqueue))
			Expect(*newRlimits.Nice).To(Equal(valNice))
			Expect(*newRlimits.Nofile).To(Equal(valNofile))
			Expect(*newRlimits.Nproc).To(Equal(valNproc))
			Expect(*newRlimits.Rss).To(Equal(valRss))
			Expect(*newRlimits.Rtprio).To(Equal(valRtprio))
			Expect(*newRlimits.Sigpending).To(Equal(valSigpending))
			Expect(*newRlimits.Stack).To(Equal(valStack))
		})
	})

	Describe("Apply", func() {
		Context("When an error occurs", func() {
			var (
				noFileValue uint64 = 1048999 // this will cause an error for number of files
			)

			JustBeforeEach(func() {
				rlimits.Nofile = &noFileValue
			})

			It("returns an error", func() {
				Expect(mgr.Apply(rlimits)).To(MatchError("container_daemon: setting rlimit: operation not permitted"))
			})
		})

		Context("Setting an RLimit", func() {
			var rLimitValue uint64

			BeforeEach(func() {
				rLimitValue = 9000
				rlimits.Core = &rLimitValue

				prevRlimit = new(syscall.Rlimit)
				err := syscall.Getrlimit(container_daemon.RLIMIT_CORE, prevRlimit)
				Expect(err).ToNot(HaveOccurred())
			})

			AfterEach(func() {
				err := syscall.Setrlimit(container_daemon.RLIMIT_CORE, prevRlimit)
				Expect(err).ToNot(HaveOccurred())
			})

			It("sets appropriate resource limit", func() {
				Expect(mgr.Apply(rlimits)).To(Succeed())
				Expect(syscall.Getrlimit(container_daemon.RLIMIT_CORE, &systemRlimits)).To(Succeed())
				Expect(systemRlimits.Cur).To(Equal(rLimitValue))
				Expect(systemRlimits.Max).To(Equal(rLimitValue))
			})
		})
	})
})
