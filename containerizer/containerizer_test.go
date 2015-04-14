package containerizer_test

import (
	"errors"
	"os"

	"github.com/cloudfoundry-incubator/garden-linux/containerizer"
	"github.com/cloudfoundry-incubator/garden-linux/containerizer/fake_container_execer"
	"github.com/cloudfoundry-incubator/garden-linux/containerizer/fake_rootfs_enterer"
	"github.com/cloudfoundry-incubator/garden-linux/containerizer/fake_set_uider"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Containerizer", func() {
	Describe("Create", func() {
		var cz *containerizer.Containerizer
		var containerExecer *fake_container_execer.FakeContainerExecer

		BeforeEach(func() {
			containerExecer = &fake_container_execer.FakeContainerExecer{}

			cz = &containerizer.Containerizer{
				Execer:      containerExecer,
				InitBinPath: "initd",
			}
		})

		It("Runs the initd process in a container", func() {
			Expect(cz.Create()).To(Succeed())
			Expect(containerExecer.ExecCallCount()).To(Equal(1))
			binPath, args := containerExecer.ExecArgsForCall(0)
			Expect(binPath).To(Equal("initd"))
			Expect(args).To(BeEmpty())
		})

		PIt("exports PID environment variable", func() {})

		Context("when execer fails", func() {
			It("returns an error", func() {
				containerExecer.ExecReturns(0, errors.New("Oh my gawsh"))
				Expect(cz.Create()).To(MatchError("containerizer: Failed to create container: Oh my gawsh"))
			})
		})
	})

	Describe("Child", func() {
		var cz *containerizer.Containerizer
		var rootFS *fake_rootfs_enterer.FakeRootFSEnterer
		var setUider *fake_set_uider.FakeSetUider
		var workingDirectory string

		BeforeEach(func() {
			var err error

			workingDirectory, err = os.Getwd()
			Expect(err).ToNot(HaveOccurred())

			rootFS = &fake_rootfs_enterer.FakeRootFSEnterer{}
			setUider = &fake_set_uider.FakeSetUider{}

			cz = &containerizer.Containerizer{
				RootFS:   rootFS,
				SetUider: setUider,
			}
		})

		AfterEach(func() {
			Expect(os.Chdir(workingDirectory)).To(Succeed())
		})

		It("enters the rootfs", func() {
			Expect(cz.Child()).To(Succeed())
			Expect(rootFS.EnterCallCount()).To(Equal(1))
		})

		It("setus uid", func() {
			Expect(cz.Child()).To(Succeed())
			Expect(setUider.SetUidCallCount()).To(Equal(1))
		})

		Context("when enter rootfs fails", func() {
			BeforeEach(func() {
				rootFS.EnterReturns(errors.New("Opps"))
			})

			It("returns an error", func() {
				Expect(cz.Child()).To(MatchError("containerizer: Failed to enter root fs: Opps"))
			})

			It("does not set uid", func() {
				cz.Child()
				Expect(setUider.SetUidCallCount()).To(Equal(0))
			})
		})

		Context("when set uid fails", func() {
			BeforeEach(func() {
				setUider.SetUidReturns(errors.New("Opps"))
			})

			It("returns an error", func() {
				Expect(cz.Child()).To(MatchError("containerizer: Failed to set uid: Opps"))
			})
		})
	})
})
