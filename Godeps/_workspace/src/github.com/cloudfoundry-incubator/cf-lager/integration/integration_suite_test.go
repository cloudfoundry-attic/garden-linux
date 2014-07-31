package main_test

import (
	"os/exec"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"

	"testing"
)

var testBinary string

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Integration Suite")
}

var _ = BeforeSuite(func() {
	var err error
	testBinary, err = gexec.Build("github.com/cloudfoundry-incubator/cf-lager/integration")
	Ω(err).ShouldNot(HaveOccurred())
})

var _ = AfterSuite(func() {
	gexec.CleanupBuildArtifacts()
})

var _ = Describe("CF-Lager", func() {
	It("provides flags", func() {
		session, err := gexec.Start(exec.Command(testBinary, "--help"), GinkgoWriter, GinkgoWriter)
		Ω(err).ShouldNot(HaveOccurred())
		session.Wait()
		Ω(session.Err.Contents()).Should(ContainSubstring("-logLevel"))
	})

	It("pipes output to stdout", func() {
		session, err := gexec.Start(exec.Command(testBinary), GinkgoWriter, GinkgoWriter)
		Ω(err).ShouldNot(HaveOccurred())
		session.Wait()

		Ω(session.Out.Contents()).Should(ContainSubstring("info"))
	})

	It("defaults to the info log level", func() {
		session, err := gexec.Start(exec.Command(testBinary), GinkgoWriter, GinkgoWriter)
		Ω(err).ShouldNot(HaveOccurred())
		session.Wait()

		Ω(session.Out.Contents()).ShouldNot(ContainSubstring("debug"))
		Ω(session.Out.Contents()).Should(ContainSubstring("info"))
		Ω(session.Out.Contents()).Should(ContainSubstring("error"))
		Ω(session.Out.Contents()).Should(ContainSubstring("fatal"))
	})

	It("honors the passed-in log level", func() {
		session, err := gexec.Start(exec.Command(testBinary, "-logLevel=debug"), GinkgoWriter, GinkgoWriter)
		Ω(err).ShouldNot(HaveOccurred())
		session.Wait()

		Ω(session.Out.Contents()).Should(ContainSubstring("debug"))
		Ω(session.Out.Contents()).Should(ContainSubstring("info"))
		Ω(session.Out.Contents()).Should(ContainSubstring("error"))
		Ω(session.Out.Contents()).Should(ContainSubstring("fatal"))
	})
})
