package container_daemon_test

import (
	"errors"
	"io/ioutil"
	"os"
	"syscall"
	"time"

	"github.com/cloudfoundry-incubator/garden-linux/container_daemon"
	"github.com/cloudfoundry-incubator/garden-linux/container_daemon/fake_poller"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/pivotal-golang/lager"
	"github.com/pivotal-golang/lager/lagertest"
)

var _ = Describe("Streamer", func() {
	var (
		reader, pipeWriter *os.File
		fakePoller         *fake_poller.FakePoller
		streamer           *container_daemon.Streamer
		buffer             *gbytes.Buffer
		testLogger         lager.Logger
	)

	BeforeEach(func() {
		var err error
		reader, pipeWriter, err = os.Pipe()
		Expect(err).ToNot(HaveOccurred())

		buffer = gbytes.NewBuffer()
		fakePoller = new(fake_poller.FakePoller)

		testLogger = lagertest.NewTestLogger("test")
	})

	JustBeforeEach(func() {
		streamer = container_daemon.NewStreamerWithPoller(reader, buffer, testLogger, fakePoller)
	})

	Describe("Streaming", func() {
		AfterEach(func() {
			streamer.Stop()
		})

		It("streams to a buffer", func() {
			Expect(streamer.Start(true)).To(Succeed())

			pipeWriter.WriteString("banana")
			Eventually(buffer, "1s").Should(gbytes.Say("banana"))

			Expect(fakePoller.PollCallCount()).To(BeNumerically(">", 0))
		})

		It("streams multiple writes", func() {
			Expect(streamer.Start(true)).To(Succeed())

			pipeWriter.WriteString("banana")
			Eventually(buffer, "1s").Should(gbytes.Say("banana"))

			pipeWriter.WriteString("apple")
			Eventually(buffer, "1s").Should(gbytes.Say("apple"))

			Expect(fakePoller.PollCallCount()).To(BeNumerically(">", 1))
		})

		It("streams more data than the buffer size", func() {
			streamer.SetBufferSize(1)
			Expect(streamer.Start(true)).To(Succeed())

			pipeWriter.WriteString("banana")
			Eventually(buffer, "1s").Should(gbytes.Say("banana"))
		})
	})

	Describe("Stop", func() {
		It("stops streaming", func() {
			Expect(streamer.Start(true)).To(Succeed())

			pipeWriter.WriteString("banana")
			Eventually(buffer, "1s").Should(gbytes.Say("banana"))

			Expect(streamer.Stop()).To(Succeed())

			pipeWriter.WriteString("apple")
			Consistently(func() string {
				return string(buffer.Contents())
			}, "1s").Should(Equal("banana"))
		})

		It("should not hang if the reader is closed", func(done Done) {
			reader.Close()

			Expect(streamer.Start(true)).To(Succeed())

			Expect(streamer.Stop()).To(Succeed())

			close(done)
		}, 5.0)

		Context("when reading fails", func() {
			BeforeEach(func() {
				var err error
				reader, err = ioutil.TempFile("", "banana")
				Expect(err).ToNot(HaveOccurred())
			})

			AfterEach(func() {
				os.Remove(reader.Name())
			})

			It("should not hang if reading fails", func(done Done) {
				Expect(streamer.Start(true)).To(Succeed())

				time.Sleep(time.Millisecond * 500)

				Expect(streamer.Stop()).To(Succeed())

				close(done)
			}, 5.0)
		})

		It("should not hang if writing fails", func(done Done) {
			Expect(streamer.Start(true)).To(Succeed())

			buffer.Close()
			pipeWriter.WriteString("banana")

			time.Sleep(time.Millisecond * 500)

			Expect(streamer.Stop()).To(Succeed())

			close(done)
		}, 5.0)

		Context("when the poller never returns", func() {
			var pollChan chan bool

			BeforeEach(func() {
				pollChan = make(chan bool)

				fakePoller.PollStub = func() error {
					<-pollChan
					return nil
				}
			})

			AfterEach(func() {
				close(pollChan)
			})

			It("reads once", func() {
				Expect(streamer.Start(true)).To(Succeed())

				pipeWriter.WriteString("banana")
				Expect(streamer.Stop()).Should(Succeed())

				Eventually(buffer, "1s").Should(gbytes.Say("banana"))
			})

		})

		Context("when the reader and writer should be closed", func() {
			It("attempts to close the reader and the writer", func() {
				Expect(streamer.Start(true)).To(Succeed())
				Expect(streamer.Stop()).To(Succeed())

				Expect(reader.Close()).To(MatchError(syscall.EINVAL))
				Expect(buffer.Closed()).To(BeTrue())
			})
		})

		Context("when the reader and writer should not be closed", func() {
			It("does not attempt to close the reader or the writer", func() {
				Expect(streamer.Start(false)).To(Succeed())
				Expect(streamer.Stop()).To(Succeed())

				Expect(reader.Close()).To(Succeed())
				Expect(buffer.Closed()).To(BeFalse())
			})
		})

	})

	Context("when the poller returns an error", func() {
		BeforeEach(func() {
			fakePoller.PollReturns(errors.New("oh no!"))
		})

		JustBeforeEach(func() {
			Expect(streamer.Start(true)).To(Succeed())
		})

		It("should not poll anymore", func() {
			Consistently(func() int {
				return fakePoller.PollCallCount()
			}, "1s").Should(BeNumerically("<=", 1))
		})
	})

	Describe("protocol errors", func() {
		Context("when streaming", func() {
			JustBeforeEach(func() {
				Expect(streamer.Start(true)).To(Succeed())
			})

			It("start returns an error", func() {
				Expect(streamer.Start(true)).To(MatchError("container_daemon: streamer already streaming"))
			})
		})

		Context("when not streaming", func() {

			It("stop returns an error", func() {
				Expect(streamer.Stop()).To(MatchError("container_daemon: streamer not streaming"))
			})
		})
	})

	Describe("Poll", func() {
		Context("when polling has been stopped", func() {
			It("closes polling channel and returns", func(done Done) {
				pollChan := make(chan bool)
				stopPollChan := make(chan bool, 1)
				stopPollChan <- true
				container_daemon.Poll(pollChan, stopPollChan, fakePoller, testLogger)
				Expect(pollChan).To(BeClosed())
				close(done)
			}, 1.0)
		})

		Context("when polling has been stopped and the polling channel is blocked", func() {
			It("closes polling channel and returns", func(done Done) {
				pollChan := make(chan bool)
				stopPollChan := make(chan bool, 1)
				go func() {
					defer GinkgoRecover()
					Eventually(fakePoller.PollCallCount()).Should(Equal(1))
					stopPollChan <- true
				}()
				container_daemon.Poll(pollChan, stopPollChan, fakePoller, testLogger)
				Expect(pollChan).To(BeClosed())
				close(done)
			}, 1.0)
		})
	})
})
