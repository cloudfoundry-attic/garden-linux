package system_test

import (
	"io/ioutil"
	"os"

	"github.com/cloudfoundry-incubator/garden-linux/system"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("MaxValidUid", func() {
	It("should return the minimum size found in the files", func() {
		uidMapPath := writeTmpFile("0 0 12455\n")
		defer os.Remove(uidMapPath)
		gidMapPath := writeTmpFile("0 0   32455\n")
		defer os.Remove(gidMapPath)

		Expect(system.MaxValidUid(uidMapPath, gidMapPath)).To(Equal(12454))

		uidMapPath = writeTmpFile("0\t0 22455\n")
		defer os.Remove(uidMapPath)
		gidMapPath = writeTmpFile("0 0 12455\n")
		defer os.Remove(gidMapPath)

		Expect(system.MaxValidUid(uidMapPath, gidMapPath)).To(Equal(12454))
	})

	Context("when a map file is invalid", func() {
		It("returns an error", func() {
			By("having multiple lines")
			uidMapPath := writeTmpFile("0 0 12455\n800000 800000 9\n")
			defer os.Remove(uidMapPath)
			gidMapPath := writeTmpFile("0 0 32455\n")
			defer os.Remove(gidMapPath)

			_, err := system.MaxValidUid(uidMapPath, gidMapPath)
			Expect(err).To(
				MatchError(HavePrefix("system: unsupported map file contents")),
			)

			By("having a non-zero host id")
			uidMapPath = writeTmpFile("0 12 12455\n")
			defer os.Remove(uidMapPath)
			gidMapPath = writeTmpFile("0 0 32455\n")
			defer os.Remove(gidMapPath)

			_, err = system.MaxValidUid(uidMapPath, gidMapPath)
			Expect(err).To(
				MatchError(HavePrefix("system: unsupported map file contents")),
			)

			By("having a non-zero container id")
			uidMapPath = writeTmpFile("0 0 12455\n")
			defer os.Remove(uidMapPath)
			gidMapPath = writeTmpFile("12 0 32455\n")
			defer os.Remove(gidMapPath)

			_, err = system.MaxValidUid(uidMapPath, gidMapPath)
			Expect(err).To(
				MatchError(HavePrefix("system: unsupported map file contents")),
			)
		})
	})
})

func writeTmpFile(contents string) string {
	tmpFile, err := ioutil.TempFile("", "")
	Expect(err).ToNot(HaveOccurred())
	defer tmpFile.Close()

	_, err = tmpFile.WriteString(contents)
	Expect(err).ToNot(HaveOccurred())

	return tmpFile.Name()
}
