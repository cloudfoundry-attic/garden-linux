package main_test

import (
	"path"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"

	"os"
	"testing"
)

var capabilityTestBin string

func TestCapability(t *testing.T) {
	SynchronizedBeforeSuite(func() []byte {
		os.Setenv("CGO_ENABLED", "0")
		defer os.Unsetenv("CGO_ENABLED")
		capabilityPath, err := gexec.Build("code.cloudfoundry.org/garden-linux/integration/helpers/capcheck", "-a", "-installsuffix", "static")
		Expect(err).ToNot(HaveOccurred())

		os.Chmod(capabilityPath, 777)
		os.Chown(capabilityPath, 0, 0)

		capabilityDir := path.Dir(capabilityPath)
		os.Chmod(capabilityDir, 777)
		os.Chown(capabilityDir, 0, 0)

		return []byte(capabilityPath)
	}, func(path []byte) {
		capabilityTestBin = string(path)
	})

	RegisterFailHandler(Fail)
	RunSpecs(t, "Capability Suite")
}
