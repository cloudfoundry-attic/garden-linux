package system_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"encoding/json"
	"testing"

	"github.com/onsi/gomega/gexec"
)

var fakeMounterBin, fakeContainerBin string

func TestSystem(t *testing.T) {
	var beforeSuite struct {
		FakeMounterPath   string
		FakeContainerPath string
	}

	SynchronizedBeforeSuite(func() []byte {
		var err error
		beforeSuite.FakeMounterPath, err = gexec.Build("github.com/cloudfoundry-incubator/garden-linux/containerizer/system/fake_mounter", "-race")
		Expect(err).ToNot(HaveOccurred())

		beforeSuite.FakeContainerPath, err = gexec.Build("github.com/cloudfoundry-incubator/garden-linux/containerizer/system/fake_container", "-race")
		Expect(err).ToNot(HaveOccurred())

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
	})

	SynchronizedAfterSuite(func() {
		//noop
	}, func() {
		gexec.CleanupBuildArtifacts()
	})

	RegisterFailHandler(Fail)
	RunSpecs(t, "System Suite")
}
