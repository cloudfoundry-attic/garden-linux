package repository_fetcher_test

import (
	"io"
	"io/ioutil"
	"strings"

	"github.com/cloudfoundry-incubator/garden-linux/shed/repository_fetcher"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("QuotaedReader", func() {
	var (
		delegate io.Reader
		quota    int64

		qr *repository_fetcher.QuotaedReader
	)

	BeforeEach(func() {
		quota = 20
	})

	JustBeforeEach(func() {
		qr = &repository_fetcher.QuotaedReader{
			R: delegate,
			N: quota,
		}
	})

	Context("when the underlying reader has less bytes than the quota", func() {
		BeforeEach(func() {
			delegate = strings.NewReader("hello")
		})

		It("reads all the data", func() {
			Expect(ioutil.ReadAll(qr)).To(Equal([]byte("hello")))
		})
	})

	Context("when the underlying reader has more bytes than the quota", func() {
		BeforeEach(func() {
			delegate = strings.NewReader("blah blah blah blah blah blah blah blah")
		})

		It("returns an error", func() {
			_, err := ioutil.ReadAll(qr)
			Expect(err).To(MatchError("quota exceeded"))
		})

		It("reads only as many bytes as allowed by the quota", func() {
			b, _ := ioutil.ReadAll(qr)
			Expect(b).To(HaveLen(int(quota)))
		})
	})
})
