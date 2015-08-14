package system_test

import (
	"io/ioutil"
	"os"

	"github.com/cloudfoundry-incubator/garden-linux/system"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("MappingList", func() {
	Context("when the mapping does not contain the given id", func() {
		It("returns the original id", func() {
			mapping := system.MappingList{}
			Expect(mapping.Map(55)).To(Equal(55))
		})
	})

	Context("when the mapping contains the given id but the range size is zero", func() {
		It("returns the original id", func() {
			mapping := system.MappingList{{
				FromID: 55,
				ToID:   77,
				Size:   0,
			}}

			Expect(mapping.Map(55)).To(Equal(55))
		})
	})

	Context("when the mapping contains the given id as the first element of a range", func() {
		It("returns the mapped id", func() {
			mapping := system.MappingList{{
				FromID: 55,
				ToID:   77,
				Size:   1,
			}}

			Expect(mapping.Map(55)).To(Equal(77))
		})
	})

	Context("when the mapping contains the given id as path of a range", func() {
		It("returns the mapped id", func() {
			mapping := system.MappingList{{
				FromID: 55,
				ToID:   77,
				Size:   10,
			}}

			Expect(mapping.Map(64)).To(Equal(86))
		})
	})

	Context("when the uid is just outside of the range of a mapping (defensive)", func() {
		It("returns the original id", func() {
			mapping := system.MappingList{{
				FromID: 55,
				ToID:   77,
				Size:   10,
			}}

			Expect(mapping.Map(65)).To(Equal(65))
		})
	})

	Describe("MaxValidUid", func() {
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
})

func writeTmpFile(contents string) string {
	tmpFile, err := ioutil.TempFile("", "")
	Expect(err).ToNot(HaveOccurred())
	defer tmpFile.Close()

	_, err = tmpFile.WriteString(contents)
	Expect(err).ToNot(HaveOccurred())

	return tmpFile.Name()
}
