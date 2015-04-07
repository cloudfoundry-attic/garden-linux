package lifecycle_test

import (
	"fmt"
	"net/http"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Garden", func() {

	var debugAddr string

	BeforeEach(func() {
		debugAddr = fmt.Sprintf("0.0.0.0:%d", 15000+GinkgoParallelNode())

	})

	It("started without --debugAddr flag", func() {
		client = startGarden()

		_, err := http.Get(fmt.Sprintf("http://%s/debug/pprof/?debug=1", debugAddr))
		Ω(err).Should(HaveOccurred())

	})

	It("started with --debugAddr flag", func() {
		client = startGarden("--debugAddr", debugAddr)

		_, err := http.Get(fmt.Sprintf("http://%s/debug/pprof/?debug=1", debugAddr))
		Ω(err).ShouldNot(HaveOccurred())

		_, err = http.Get(fmt.Sprintf("http://%s/log-level -X PUT -d debug", debugAddr))
		Ω(err).ShouldNot(HaveOccurred())

		_, err = http.Get(fmt.Sprintf("http://%s/log-level -X PUT -d info", debugAddr))
		Ω(err).ShouldNot(HaveOccurred())

		_, err = http.Get(fmt.Sprintf("http://%s/log-level -X PUT -d error", debugAddr))
		Ω(err).ShouldNot(HaveOccurred())

		_, err = http.Get(fmt.Sprintf("http://%s/log-level -X PUT -d fatal", debugAddr))
		Ω(err).ShouldNot(HaveOccurred())
	})

})
