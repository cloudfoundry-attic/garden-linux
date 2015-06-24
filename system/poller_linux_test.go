package system_test

import (
	"github.com/cloudfoundry-incubator/garden-linux/system"

	"os"
	"sync"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Poller", func() {

	var (
		r, w      *os.File
		poller    *system.Poller
		pollChan  chan bool
		done      bool
		doneMutex sync.Mutex
	)

	BeforeEach(func() {
		var err error
		r, w, err = os.Pipe()
		Expect(err).NotTo(HaveOccurred())

		poller = system.NewPoller([]uintptr{r.Fd()})

		pollChan = make(chan bool)

		doneMutex.Lock()
		done = false
		doneMutex.Unlock()

		go func(poller *system.Poller, pollChan chan bool) {
			defer func() {
				GinkgoRecover()
				close(pollChan)
			}()

			for {
				if poller.Poll() == nil {
					doneMutex.Lock()
					if done {
						doneMutex.Unlock()
						return
					}
					doneMutex.Unlock()

					pollChan <- true
				} else {
					return
				}
			}
		}(poller, pollChan)
	})

	AfterEach(func() {
		doneMutex.Lock()
		done = true
		doneMutex.Unlock()

		r.Close()
		w.Close()
	})

	It("should return when there is data to read", func() {
		w.WriteString("x")
		Eventually(pollChan, "1s").Should(Receive(Equal(true)))
	})

	It("should not return when there is no data to read", func() {
		Eventually(pollChan, "1s").ShouldNot(Receive())
	})

	It("should return when file is closed", func() {
		w.Close()
		Eventually(pollChan, "1s").Should(Receive(Equal(true)))
	})

	Context("when polling has already happened", func() {
		BeforeEach(func() {
			w.WriteString("x")
			Eventually(pollChan, "1s").Should(Receive(Equal(true)))
			buf := make([]byte, 1024)
			r.Read(buf)
		})

		It("should return when there is data to read", func() {
			w.WriteString("x")
			Eventually(pollChan, "1s").Should(Receive(Equal(true)))
		})

		It("should not return when there is no data to read", func() {
			Eventually(pollChan, "1s").ShouldNot(Receive())
		})

		It("should return when file is closed", func() {
			w.Close()
			Eventually(pollChan, "1s").Should(Receive(Equal(true)))
		})
	})
})
