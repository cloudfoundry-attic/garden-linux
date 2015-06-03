package system_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"

	"github.com/cloudfoundry-incubator/garden-linux/containerizer/system"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Unmount", func() {

	It("unmounts the specified directory", func() {
		dir, err := ioutil.TempDir("", "")
		file := filepath.Join(dir, "file")
		Expect(err).ToNot(HaveOccurred())
		Expect(syscall.Mount("", dir, "tmpfs", 0, "")).To(Succeed())
		Expect(ioutil.WriteFile(file, []byte("hi"), os.ModePerm)).To(Succeed())

		um := &system.Unmount{dir}
		Expect(um.Unmount()).To(Succeed())
		Expect(file).ToNot(BeAnExistingFile())
	})
})
