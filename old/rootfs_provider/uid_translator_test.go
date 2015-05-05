package rootfs_provider

import (
	"os"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("UidTranslator", func() {
	var translator *UidTranslator
	var chowned []chown

	BeforeEach(func() {
		uidMap := fakeMap{
			From: 12,
			To:   24,
		}

		gidMap := fakeMap{
			From: 33,
			To:   66,
		}

		translator = NewUidTranslator(uidMap, gidMap)
		translator.getuidgid = func(info os.FileInfo) (int, int, error) {
			return info.(fakeInfo).uid, info.(fakeInfo).gid, nil
		}

		translator.chown = func(path string, uid, gid int) error {
			chowned = append(chowned, chown{
				path: path,
				uid:  uid,
				gid:  gid,
			})

			return nil
		}
	})

	Context("when neither mapping affects the file", func() {
		It("preserves original files", func() {
			Expect(translator.Translate("some-path", fakeInfo{1, 2}, nil)).To(Succeed())
			Expect(chowned).To(BeEmpty())
		})
	})

	Context("when only the uid is mapped", func() {
		It("changes only the uid", func() {
			Expect(translator.Translate("some-path", fakeInfo{12, 2}, nil)).To(Succeed())
			Expect(chowned).To(ContainElement(chown{
				path: "some-path",
				uid:  24,
				gid:  2,
			}))
		})
	})

	Context("when only the gid is mapped", func() {
		It("changes only the gid", func() {
			Expect(translator.Translate("some-path", fakeInfo{2, 33}, nil)).To(Succeed())
			Expect(chowned).To(ContainElement(chown{
				path: "some-path",
				uid:  2,
				gid:  66,
			}))
		})
	})

	Context("when both uid and gid are mapped", func() {
		It("changes both", func() {
			Expect(translator.Translate("some-path", fakeInfo{12, 33}, nil)).To(Succeed())
			Expect(chowned).To(ContainElement(chown{
				path: "some-path",
				uid:  24,
				gid:  66,
			}))
		})
	})
})

type chown struct {
	path string
	uid  int
	gid  int
}

type fakeInfo struct {
	uid int
	gid int
}

func (f fakeInfo) Name() string {
	return ""
}

func (f fakeInfo) Size() int64 {
	return 0
}
func (f fakeInfo) Mode() os.FileMode {
	return 0
}
func (f fakeInfo) ModTime() time.Time {
	return time.Now()
}
func (f fakeInfo) IsDir() bool {
	return false
}
func (f fakeInfo) Sys() interface{} {
	return nil
}

type fakeMap struct {
	From int
	To   int
}

func (f fakeMap) Map(id int) int {
	if id == f.From {
		return f.To
	}

	return id
}
