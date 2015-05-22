package container_daemon_test

import (
	"math"
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
			It("sets hard rlimit "+name, func() {
				Expect(syscall.Getrlimit(id, &systemRlimits)).To(Succeed())
				Expect(systemRlimits.Max).To(Equal(val))
			})
		}

		BeforeEach(func() {
			Expect(mgr.Init()).To(Succeed())
		})

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

			env := mgr.EncodeEnv(rlimits)
			Expect(env).To(HaveLen(15))

			newRlimits := mgr.DecodeEnv(env)
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
				rLimitValue1 uint64 = 1000
				rLimitStack  uint64 = 100000
				noFileValue  uint64 = 1048999 // this will cause an error for number of files
			)

			JustBeforeEach(func() {
				rlimits.Cpu = &rLimitValue1
				rlimits.Fsize = &rLimitValue1
				rlimits.Data = &rLimitValue1
				rlimits.Stack = &rLimitStack
				rlimits.Core = &rLimitValue1
				rlimits.Nofile = &noFileValue
			})

			It("returns an error", func() {
				Expect(mgr.Apply(rlimits)).To(MatchError("container_daemon: setting rlimit: operation not permitted"))
			})
		})

		Context("CPU limit", func() {
			var rLimitValue uint64 = 1200

			BeforeEach(func() {
				rlimits.Cpu = &rLimitValue
				prevRlimit = new(syscall.Rlimit)

				err := syscall.Getrlimit(container_daemon.RLIMIT_CPU, prevRlimit)
				Expect(err).ToNot(HaveOccurred())
			})

			AfterEach(func() {
				err := syscall.Setrlimit(container_daemon.RLIMIT_CPU, prevRlimit)
				Expect(err).ToNot(HaveOccurred())
			})

			It("sets appropriate CPU resource limit", func() {
				Expect(mgr.Apply(rlimits)).To(Succeed())
				Expect(syscall.Getrlimit(container_daemon.RLIMIT_CPU, &systemRlimits)).To(Succeed())
				Expect(systemRlimits.Cur).To(Equal(rLimitValue))
				Expect(systemRlimits.Max).To(Equal(rLimitValue))
			})
		})

		Context("FSIZE limit", func() {
			var rLimitValue uint64 = 1200

			BeforeEach(func() {
				rlimits.Fsize = &rLimitValue
				prevRlimit = new(syscall.Rlimit)

				err := syscall.Getrlimit(container_daemon.RLIMIT_FSIZE, prevRlimit)
				Expect(err).ToNot(HaveOccurred())
			})

			AfterEach(func() {
				err := syscall.Setrlimit(container_daemon.RLIMIT_FSIZE, prevRlimit)
				Expect(err).ToNot(HaveOccurred())
			})

			It("sets appropriate FSIZE resource limit", func() {
				Expect(mgr.Apply(rlimits)).To(Succeed())
				Expect(syscall.Getrlimit(container_daemon.RLIMIT_FSIZE, &systemRlimits)).To(Succeed())
				Expect(systemRlimits.Cur).To(Equal(rLimitValue))
				Expect(systemRlimits.Max).To(Equal(rLimitValue))
			})
		})

		Context("DATA limit", func() {
			var rLimitValue uint64 = 1200

			BeforeEach(func() {
				rlimits.Data = &rLimitValue
				prevRlimit = new(syscall.Rlimit)

				err := syscall.Getrlimit(container_daemon.RLIMIT_DATA, prevRlimit)
				Expect(err).ToNot(HaveOccurred())
			})

			AfterEach(func() {
				err := syscall.Setrlimit(container_daemon.RLIMIT_DATA, prevRlimit)
				Expect(err).ToNot(HaveOccurred())
			})

			It("sets appropriate DATA resource limit", func() {
				Expect(mgr.Apply(rlimits)).To(Succeed())
				Expect(syscall.Getrlimit(container_daemon.RLIMIT_DATA, &systemRlimits)).To(Succeed())
				Expect(systemRlimits.Cur).To(Equal(rLimitValue))
				Expect(systemRlimits.Max).To(Equal(rLimitValue))
			})
		})

		Context("STACK limit", func() {
			var rLimitValue uint64

			BeforeEach(func() {
				// Allowed limit is 2^23 (8388608)
				rLimitValue = uint64(math.Pow(2, 23)) + 1000
				rlimits.Stack = &rLimitValue

				prevRlimit = new(syscall.Rlimit)
				err := syscall.Getrlimit(container_daemon.RLIMIT_STACK, prevRlimit)
				Expect(err).ToNot(HaveOccurred())
			})

			AfterEach(func() {
				err := syscall.Setrlimit(container_daemon.RLIMIT_STACK, prevRlimit)
				Expect(err).ToNot(HaveOccurred())
			})

			It("sets appropriate STACK resource limit", func() {
				Expect(mgr.Apply(rlimits)).To(Succeed())
				Expect(syscall.Getrlimit(container_daemon.RLIMIT_STACK, &systemRlimits)).To(Succeed())
				Expect(systemRlimits.Cur).To(Equal(rLimitValue))
				Expect(systemRlimits.Max).To(Equal(rLimitValue))
			})
		})

		Context("CORE limit", func() {
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

			It("sets appropriate CORE resource limit", func() {
				Expect(mgr.Apply(rlimits)).To(Succeed())
				Expect(syscall.Getrlimit(container_daemon.RLIMIT_CORE, &systemRlimits)).To(Succeed())
				Expect(systemRlimits.Cur).To(Equal(rLimitValue))
				Expect(systemRlimits.Max).To(Equal(rLimitValue))
			})
		})

		Context("RSS limit", func() {
			var rLimitValue uint64

			BeforeEach(func() {
				rLimitValue = 9000
				rlimits.Rss = &rLimitValue

				prevRlimit = new(syscall.Rlimit)
				err := syscall.Getrlimit(container_daemon.RLIMIT_RSS, prevRlimit)
				Expect(err).ToNot(HaveOccurred())
			})

			AfterEach(func() {
				err := syscall.Setrlimit(container_daemon.RLIMIT_RSS, prevRlimit)
				Expect(err).ToNot(HaveOccurred())
			})

			It("sets appropriate RSS resource limit", func() {
				Expect(mgr.Apply(rlimits)).To(Succeed())
				Expect(syscall.Getrlimit(container_daemon.RLIMIT_RSS, &systemRlimits)).To(Succeed())
				Expect(systemRlimits.Cur).To(Equal(rLimitValue))
				Expect(systemRlimits.Max).To(Equal(rLimitValue))
			})
		})

		Context("NPROC limit", func() {
			var rLimitValue uint64

			BeforeEach(func() {
				rLimitValue = 500
				rlimits.Nproc = &rLimitValue

				prevRlimit = new(syscall.Rlimit)
				err := syscall.Getrlimit(container_daemon.RLIMIT_NPROC, prevRlimit)
				Expect(err).ToNot(HaveOccurred())
			})

			AfterEach(func() {
				err := syscall.Setrlimit(container_daemon.RLIMIT_NPROC, prevRlimit)
				Expect(err).ToNot(HaveOccurred())
			})

			It("sets appropriate NPROC resource limit", func() {
				Expect(mgr.Apply(rlimits)).To(Succeed())
				Expect(syscall.Getrlimit(container_daemon.RLIMIT_NPROC, &systemRlimits)).To(Succeed())
				Expect(systemRlimits.Cur).To(Equal(rLimitValue))
				Expect(systemRlimits.Max).To(Equal(rLimitValue))
			})
		})

		Context("NOFILE limit", func() {
			var rLimitValue uint64

			BeforeEach(func() {
				rLimitValue = 800
				rlimits.Nofile = &rLimitValue

				prevRlimit = new(syscall.Rlimit)
				err := syscall.Getrlimit(container_daemon.RLIMIT_NOFILE, prevRlimit)
				Expect(err).ToNot(HaveOccurred())
			})

			AfterEach(func() {
				err := syscall.Setrlimit(container_daemon.RLIMIT_NOFILE, prevRlimit)
				Expect(err).ToNot(HaveOccurred())
			})

			It("sets appropriate NOFILE resource limit", func() {
				Expect(mgr.Apply(rlimits)).To(Succeed())
				Expect(syscall.Getrlimit(container_daemon.RLIMIT_NOFILE, &systemRlimits)).To(Succeed())
				Expect(systemRlimits.Cur).To(Equal(rLimitValue))
				Expect(systemRlimits.Max).To(Equal(rLimitValue))
			})
		})

		Context("MEMLOCK limit", func() {
			var rLimitValue uint64

			BeforeEach(func() {
				rLimitValue = 1024
				rlimits.Memlock = &rLimitValue

				prevRlimit = new(syscall.Rlimit)
				err := syscall.Getrlimit(container_daemon.RLIMIT_MEMLOCK, prevRlimit)
				Expect(err).ToNot(HaveOccurred())
			})

			AfterEach(func() {
				err := syscall.Setrlimit(container_daemon.RLIMIT_MEMLOCK, prevRlimit)
				Expect(err).ToNot(HaveOccurred())
			})

			It("sets appropriate MEMLOCK resource limit", func() {
				Expect(mgr.Apply(rlimits)).To(Succeed())
				Expect(syscall.Getrlimit(container_daemon.RLIMIT_MEMLOCK, &systemRlimits)).To(Succeed())
				Expect(systemRlimits.Cur).To(Equal(rLimitValue))
				Expect(systemRlimits.Max).To(Equal(rLimitValue))
			})
		})

		Context("AS limit", func() {
			var rLimitValue uint64

			BeforeEach(func() {
				rLimitValue = 1024
				rlimits.As = &rLimitValue

				prevRlimit = new(syscall.Rlimit)
				err := syscall.Getrlimit(container_daemon.RLIMIT_AS, prevRlimit)
				Expect(err).ToNot(HaveOccurred())
			})

			AfterEach(func() {
				err := syscall.Setrlimit(container_daemon.RLIMIT_AS, prevRlimit)
				Expect(err).ToNot(HaveOccurred())
			})

			It("sets appropriate AS resource limit", func() {
				Expect(mgr.Apply(rlimits)).To(Succeed())
				Expect(syscall.Getrlimit(container_daemon.RLIMIT_AS, &systemRlimits)).To(Succeed())
				Expect(systemRlimits.Cur).To(Equal(rLimitValue))
				Expect(systemRlimits.Max).To(Equal(rLimitValue))
			})
		})

		Context("LOCKS limit", func() {
			var rLimitValue uint64

			BeforeEach(func() {
				rLimitValue = 500
				rlimits.Locks = &rLimitValue

				prevRlimit = new(syscall.Rlimit)
				err := syscall.Getrlimit(container_daemon.RLIMIT_LOCKS, prevRlimit)
				Expect(err).ToNot(HaveOccurred())
			})

			AfterEach(func() {
				err := syscall.Setrlimit(container_daemon.RLIMIT_LOCKS, prevRlimit)
				Expect(err).ToNot(HaveOccurred())
			})

			It("sets appropriate LOCKS resource limit", func() {
				Expect(mgr.Apply(rlimits)).To(Succeed())
				Expect(syscall.Getrlimit(container_daemon.RLIMIT_LOCKS, &systemRlimits)).To(Succeed())
				Expect(systemRlimits.Cur).To(Equal(rLimitValue))
				Expect(systemRlimits.Max).To(Equal(rLimitValue))
			})
		})

		Context("SIGPENDING limit", func() {
			var rLimitValue uint64

			BeforeEach(func() {
				rLimitValue = 50000
				rlimits.Sigpending = &rLimitValue

				prevRlimit = new(syscall.Rlimit)
				err := syscall.Getrlimit(container_daemon.RLIMIT_SIGPENDING, prevRlimit)
				Expect(err).ToNot(HaveOccurred())
			})

			AfterEach(func() {
				err := syscall.Setrlimit(container_daemon.RLIMIT_SIGPENDING, prevRlimit)
				Expect(err).ToNot(HaveOccurred())
			})

			It("sets appropriate SIGPENDING resource limit", func() {
				Expect(mgr.Apply(rlimits)).To(Succeed())
				Expect(syscall.Getrlimit(container_daemon.RLIMIT_SIGPENDING, &systemRlimits)).To(Succeed())
				Expect(systemRlimits.Cur).To(Equal(rLimitValue))
				Expect(systemRlimits.Max).To(Equal(rLimitValue))
			})
		})

		Context("MSGQUEUE limit", func() {
			var rLimitValue uint64

			BeforeEach(func() {
				rLimitValue = 500000
				rlimits.Msgqueue = &rLimitValue

				prevRlimit = new(syscall.Rlimit)
				err := syscall.Getrlimit(container_daemon.RLIMIT_MSGQUEUE, prevRlimit)
				Expect(err).ToNot(HaveOccurred())
			})

			AfterEach(func() {
				err := syscall.Setrlimit(container_daemon.RLIMIT_MSGQUEUE, prevRlimit)
				Expect(err).ToNot(HaveOccurred())
			})

			It("sets appropriate MSGQUEUE resource limit", func() {
				Expect(mgr.Apply(rlimits)).To(Succeed())
				Expect(syscall.Getrlimit(container_daemon.RLIMIT_MSGQUEUE, &systemRlimits)).To(Succeed())
				Expect(systemRlimits.Cur).To(Equal(rLimitValue))
				Expect(systemRlimits.Max).To(Equal(rLimitValue))
			})
		})

		Context("NICE limit", func() {
			var rLimitValue uint64

			BeforeEach(func() {
				rLimitValue = 0
				rlimits.Nice = &rLimitValue

				prevRlimit = new(syscall.Rlimit)
				err := syscall.Getrlimit(container_daemon.RLIMIT_NICE, prevRlimit)
				Expect(err).ToNot(HaveOccurred())
			})

			AfterEach(func() {
				err := syscall.Setrlimit(container_daemon.RLIMIT_NICE, prevRlimit)
				Expect(err).ToNot(HaveOccurred())
			})

			It("sets appropriate NICE resource limit", func() {
				Expect(mgr.Apply(rlimits)).To(Succeed())
				Expect(syscall.Getrlimit(container_daemon.RLIMIT_NICE, &systemRlimits)).To(Succeed())
				Expect(systemRlimits.Cur).To(Equal(rLimitValue))
				Expect(systemRlimits.Max).To(Equal(rLimitValue))
			})
		})

		Context("RTPRIO limit", func() {
			var rLimitValue uint64

			BeforeEach(func() {
				rLimitValue = 500
				rlimits.Rtprio = &rLimitValue

				prevRlimit = new(syscall.Rlimit)
				err := syscall.Getrlimit(container_daemon.RLIMIT_RTPRIO, prevRlimit)
				Expect(err).ToNot(HaveOccurred())
			})

			AfterEach(func() {
				err := syscall.Setrlimit(container_daemon.RLIMIT_RTPRIO, prevRlimit)
				Expect(err).ToNot(HaveOccurred())
			})

			It("sets appropriate RTPRIO resource limit", func() {
				Expect(mgr.Apply(rlimits)).To(Succeed())
				Expect(syscall.Getrlimit(container_daemon.RLIMIT_RTPRIO, &systemRlimits)).To(Succeed())
				Expect(systemRlimits.Cur).To(Equal(rLimitValue))
				Expect(systemRlimits.Max).To(Equal(rLimitValue))
			})
		})
	})
})
