package debug_test

import (
	"expvar"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"

	"github.com/cloudfoundry-incubator/garden-linux/debug"
	"github.com/pivotal-golang/lager"
	"github.com/tedsuo/ifrit"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Debug", func() {
	var (
		backingStorePath string
		depotPath        string
		serverProc       ifrit.Process
	)

	BeforeEach(func() {
		var err error

		backingStorePath, err = ioutil.TempDir("", "backing_stores")
		Expect(err).NotTo(HaveOccurred())
		Expect(ioutil.WriteFile(
			filepath.Join(backingStorePath, "bs-1"), []byte("test"), 0660,
		)).To(Succeed())
		Expect(ioutil.WriteFile(
			filepath.Join(backingStorePath, "bs-2"), []byte("test"), 0660,
		)).To(Succeed())

		depotPath, err = ioutil.TempDir("", "depotDirs")
		Expect(err).NotTo(HaveOccurred())
		Expect(os.Mkdir(filepath.Join(depotPath, "depot-1"), 0660)).To(Succeed())
		Expect(os.Mkdir(filepath.Join(depotPath, "depot-2"), 0660)).To(Succeed())
		Expect(os.Mkdir(filepath.Join(depotPath, "depot-3"), 0660)).To(Succeed())

		sink := lager.NewReconfigurableSink(lager.NewWriterSink(GinkgoWriter, lager.DEBUG), lager.DEBUG)
		serverProc, err = debug.Run("127.0.0.1:5123", sink, backingStorePath, depotPath)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		serverProc.Signal(os.Kill)

		Expect(os.RemoveAll(depotPath)).To(Succeed())
		Expect(os.RemoveAll(backingStorePath)).To(Succeed())
	})

	It("should report the number of loop devices, backing store files and depotDirs", func() {
		resp, err := http.Get("http://127.0.0.1:5123/debug/vars")
		Expect(err).ToNot(HaveOccurred())

		defer resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		Expect(expvar.Get("loopDevices")).NotTo(BeNil())
		Expect(expvar.Get("backingStores")).NotTo(BeNil())
		Expect(expvar.Get("backingStores").String()).To(Equal("2"))
		Expect(expvar.Get("depotDirs")).NotTo(BeNil())
		Expect(expvar.Get("depotDirs").String()).To(Equal("3"))
	})
})
