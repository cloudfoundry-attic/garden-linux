package system_test

import (
	"io/ioutil"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"encoding/json"
	"testing"

	"github.com/onsi/gomega/gexec"
)

var (
	fakeMounterBin   string
	fakeContainerBin string
	tempDirPath      string
)

func TestSystem(t *testing.T) {
	var beforeSuite struct {
		FakeMounterPath   string
		FakeContainerPath string
		TempDirPath       string
	}

	SynchronizedBeforeSuite(func() []byte {
		var err error
		beforeSuite.FakeMounterPath, err = gexec.Build("github.com/cloudfoundry-incubator/garden-linux/containerizer/system/fake_mounter", "-race")
		Expect(err).ToNot(HaveOccurred())

		beforeSuite.FakeContainerPath, err = gexec.Build("github.com/cloudfoundry-incubator/garden-linux/containerizer/system/fake_container", "-race")
		Expect(err).ToNot(HaveOccurred())

		beforeSuite.TempDirPath, err = ioutil.TempDir("", "system-tempdir")
		Expect(err).NotTo(HaveOccurred())

		b, err := json.Marshal(beforeSuite)
		Expect(err).ToNot(HaveOccurred())

		return b
	}, func(paths []byte) {
		err := json.Unmarshal(paths, &beforeSuite)
		Expect(err).ToNot(HaveOccurred())

		fakeMounterBin = beforeSuite.FakeMounterPath
		Expect(fakeMounterBin).NotTo(BeEmpty())

		fakeContainerBin = beforeSuite.FakeContainerPath
		Expect(fakeContainerBin).NotTo(BeEmpty())

		tempDirPath = beforeSuite.TempDirPath
		Expect(tempDirPath).NotTo(BeEmpty())
	})

	SynchronizedAfterSuite(func() {
		//noop
	}, func() {
		gexec.CleanupBuildArtifacts()

		// Cleaning up this directoy causes the VM to hang occasionally.
		// Expect(os.RemoveAll(tempDirPath)).To(Succeed())
	})

	RegisterFailHandler(Fail)
	RunSpecs(t, "System Suite")
}
