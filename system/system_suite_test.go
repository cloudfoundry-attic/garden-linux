package system_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"

	"testing"
)

var (
	testCapabilitiesPath string
)

type CompiledAssets struct {
	TestCapabilitiesPath string
}

var _ = SynchronizedBeforeSuite(func() []byte {
	var err error
	assets := CompiledAssets{}
	assets.TestCapabilitiesPath, err = gexec.Build("github.com/cloudfoundry-incubator/garden-linux/system/test_capabilities")
	Expect(err).ToNot(HaveOccurred())

	marshalledAssets, err := json.Marshal(assets)
	Expect(err).ToNot(HaveOccurred())
	return marshalledAssets
}, func(marshalledAssets []byte) {
	assets := CompiledAssets{}
	err := json.Unmarshal(marshalledAssets, &assets)
	Expect(err).ToNot(HaveOccurred())
	testCapabilitiesPath = assets.TestCapabilitiesPath
})

var _ = SynchronizedAfterSuite(func() {
	//noop
}, func() {
	gexec.CleanupBuildArtifacts()
})

func TestSystem(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "System Suite")
}
