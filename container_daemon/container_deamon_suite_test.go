package container_daemon_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"encoding/json"
	"testing"

	"github.com/onsi/gomega/gexec"
)

var wshBin string
var procStarterBin string

func TestContainerDeamon(t *testing.T) {
	var beforeSuite struct {
		WshPath         string
		ProcStarterPath string
	}

	SynchronizedBeforeSuite(func() []byte {
		var err error
		beforeSuite.WshPath, err = gexec.Build("github.com/cloudfoundry-incubator/garden-linux/container_daemon/wsh", "-race")
		Expect(err).ToNot(HaveOccurred())

		beforeSuite.ProcStarterPath, err = gexec.Build("github.com/cloudfoundry-incubator/garden-linux/container_daemon/proc_starter", "-race")
		Expect(err).ToNot(HaveOccurred())

		b, err := json.Marshal(beforeSuite)
		Expect(err).ToNot(HaveOccurred())

		return b
	}, func(paths []byte) {
		err := json.Unmarshal(paths, &beforeSuite)
		Expect(err).ToNot(HaveOccurred())

		wshBin = beforeSuite.WshPath
		Expect(wshBin).NotTo(BeNil())

		procStarterBin = beforeSuite.ProcStarterPath
		Expect(procStarterBin).NotTo(BeNil())
	})

	SynchronizedAfterSuite(func() {
		//noop
	}, func() {
		gexec.CleanupBuildArtifacts()
	})

	RegisterFailHandler(Fail)
	RunSpecs(t, "ContainerDeamon Suite")
}
