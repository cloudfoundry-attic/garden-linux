package cf_debug_server_test

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"strconv"

	cf_debug_server "github.com/cloudfoundry-incubator/cf-debug-server"
	"github.com/pivotal-golang/lager"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/ginkgomon"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("CF Debug Server", func() {
	var (
		logBuf *gbytes.Buffer
		sink   *lager.ReconfigurableSink

		process ifrit.Process
	)

	BeforeEach(func() {
		logBuf = gbytes.NewBuffer()
		sink = lager.NewReconfigurableSink(
			lager.NewWriterSink(logBuf, lager.DEBUG),
			// permit no logging by default, for log reconfiguration below
			lager.FATAL+1,
		)
	})

	AfterEach(func() {
		ginkgomon.Interrupt(process)
	})

	Describe("AddFlags", func() {
		It("adds flags to the flagset", func() {
			flags := flag.NewFlagSet("test", flag.ContinueOnError)
			cf_debug_server.AddFlags(flags)

			f := flags.Lookup(cf_debug_server.DebugFlag)
			Expect(f).NotTo(BeNil())
		})
	})

	Describe("DebugAddress", func() {
		Context("when flags are not added", func() {
			It("returns the empty string", func() {
				flags := flag.NewFlagSet("test", flag.ContinueOnError)
				Expect(cf_debug_server.DebugAddress(flags)).To(Equal(""))
			})
		})

		Context("when flags are added", func() {
			var flags *flag.FlagSet
			BeforeEach(func() {
				flags = flag.NewFlagSet("test", flag.ContinueOnError)
				cf_debug_server.AddFlags(flags)
			})

			Context("when set", func() {
				It("returns the address", func() {
					flags.Parse([]string{"-debugAddr", address})

					Expect(cf_debug_server.DebugAddress(flags)).To(Equal(address))
				})
			})

			Context("when not set", func() {
				It("returns the empty string", func() {
					Expect(cf_debug_server.DebugAddress(flags)).To(Equal(""))
				})
			})
		})
	})

	Describe("Run", func() {
		It("serves debug information", func() {
			var err error
			process, err = cf_debug_server.Run(address, sink)
			Expect(err).NotTo(HaveOccurred())

			debugResponse, err := http.Get(fmt.Sprintf("http://%s/debug/pprof/goroutine", address))
			Expect(err).NotTo(HaveOccurred())

			debugInfo, err := ioutil.ReadAll(debugResponse.Body)
			Expect(err).NotTo(HaveOccurred())

			Expect(debugInfo).To(ContainSubstring("goroutine profile: total"))

		})

		Context("when the address is already in use", func() {
			var listener net.Listener

			BeforeEach(func() {
				var err error
				listener, err = net.Listen("tcp", address)
				Expect(err).NotTo(HaveOccurred())
			})

			AfterEach(func() {
				listener.Close()
			})

			It("returns an error", func() {
				var err error
				process, err = cf_debug_server.Run(address, sink)
				Expect(err).To(HaveOccurred())
				Expect(err).To(BeAssignableToTypeOf(&net.OpError{}))
				netErr := err.(*net.OpError)
				Expect(netErr.Op).To(Equal("listen"))
			})
		})

		Context("checking log-level endpoint", func() {
			validForms := map[lager.LogLevel][]string{
				lager.DEBUG: []string{"debug", "DEBUG", "d", strconv.Itoa(int(lager.DEBUG))},
				lager.INFO:  []string{"info", "INFO", "i", strconv.Itoa(int(lager.INFO))},
				lager.ERROR: []string{"error", "ERROR", "e", strconv.Itoa(int(lager.ERROR))},
				lager.FATAL: []string{"fatal", "FATAL", "f", strconv.Itoa(int(lager.FATAL))},
			}

			//This will add another 16 unit tests to the suit
			for level, acceptedForms := range validForms {
				for _, form := range acceptedForms {
					testLevel := level
					testForm := form

					It("can reconfigure the given sink with "+form, func() {
						var err error
						process, err = cf_debug_server.Run(address, sink)
						Expect(err).NotTo(HaveOccurred())

						sink.Log(testLevel, []byte("hello before level change"))
						Eventually(logBuf).ShouldNot(gbytes.Say("hello before level change"))

						request, err := http.NewRequest("PUT", fmt.Sprintf("http://%s/log-level", address), bytes.NewBufferString(testForm))

						Expect(err).NotTo(HaveOccurred())

						response, err := http.DefaultClient.Do(request)
						Expect(err).NotTo(HaveOccurred())

						Expect(response.StatusCode).To(Equal(http.StatusOK))
						response.Body.Close()

						sink.Log(testLevel, []byte("Logs sent with log-level "+testForm))
						Eventually(logBuf).Should(gbytes.Say("Logs sent with log-level " + testForm))
					})
				}
			}
		})

	})
})
