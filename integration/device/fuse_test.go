package device_test

import (
	"fmt"
	"os/exec"
	"path/filepath"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("Fuse", func() {
	var container garden.Container
	var privilegedContainer bool
	var privilegedProcess bool

	BeforeEach(func() {
		privilegedContainer = true
		privilegedProcess = true
	})

	JustBeforeEach(func() {
		client = startGarden()

		var err error
		container, err = client.Create(garden.ContainerSpec{
			RootFSPath: fuseRootFSPath,
			Privileged: privilegedContainer,
		})
		Ω(err).ShouldNot(HaveOccurred())
	})

	Describe("/dev/fuse", func() {
		It("is a character special device file", func() {
			process, err := container.Run(garden.ProcessSpec{
				Privileged: privilegedProcess,
				Path:       "/usr/bin/test",
				Args:       []string{"-c", "/dev/fuse"},
			}, garden.ProcessIO{Stdout: GinkgoWriter, Stderr: GinkgoWriter})
			Ω(err).ShouldNot(HaveOccurred())

			Ω(process.Wait()).Should(Equal(0), "/dev/fuse cannot be found or is not a character special device.")
		})

		Context("in a privileged Container", func() {
			BeforeEach(func() {
				privilegedContainer = true
			})

			Context("a privileged process", func() {
				BeforeEach(func() {
					privilegedProcess = true
				})

				It("can mount a fuse filesystem", func() {
					canCreateAndUseFuseFileSystem(container, privilegedProcess)
				})
			})

			Context("a non-privileged process", func() {
				BeforeEach(func() {
					privilegedProcess = false
				})

				It("can mount a fuse filesystem", func() {
					canCreateAndUseFuseFileSystem(container, privilegedProcess)
				})
			})
		})

		Context("when running as host root in an unprivliged container", func() {
			BeforeEach(func() {
				privilegedContainer = false
			})

			Context("a privileged process", func() {
				BeforeEach(func() {
					privilegedProcess = true
				})

				FIt("can mount a fuse filesystem", func() {
					cmd, err := gexec.Start(exec.Command("sh", "-c", fmt.Sprintf(`
cd /tmp
curl https://www.kernel.org/pub/linux/utils/util-linux/v2.24/util-linux-2.24.tar.gz | tar -zxf-
cd util-linux-2.24
./configure --without-ncurses
make nsenter
cp nsenter /usr/local/bin
WSHDP=$(ps -aef | grep wshd | grep "%s" | head -n 1 | awk '{print $2}')
echo $WSHDP
echo $(which fusermount)
FUSERPATH=$(nsenter -t $WSHDP -m -S 0 -G 0 -- which fusermount)
nsenter -t $WSHDP -m -S 0 -G 0 -- chmod ugo+rws $FUSERPATH
nsenter -t $WSHDP -m -S 0 -G 0 -- mkdir -p /fuse-test
nsenter -t $WSHDP -m -S 0 -G 0 -- cat /etc/fuse.conf
nsenter -t $WSHDP -m -S 0 -G 0 -- /usr/bin/hellofs /fuse-test
				`, container.Handle())), GinkgoWriter, GinkgoWriter)
					Ω(err).ShouldNot(HaveOccurred())

					Eventually(cmd, "60s").Should(gexec.Exit(0))
					//canCreateAndUseFuseFileSystem(container, privilegedProcess)
				})
			})

			Context("a non-privileged process", func() {
				BeforeEach(func() {
					privilegedProcess = false
				})

				It("can mount a fuse filesystem", func() {
					canCreateAndUseFuseFileSystem(container, privilegedProcess)
				})
			})
		})
	})
})

func canCreateAndUseFuseFileSystem(container garden.Container, privilegedProcess bool) {
	mountpoint := "/tmp/fuse-test"

	process, err := container.Run(garden.ProcessSpec{
		Privileged: privilegedProcess,
		Path:       "mkdir",
		Args:       []string{"-p", mountpoint},
	}, garden.ProcessIO{Stdout: GinkgoWriter, Stderr: GinkgoWriter})
	Ω(err).ShouldNot(HaveOccurred())
	Ω(process.Wait()).Should(Equal(0), "Could not make temporary directory!")

	process, err = container.Run(garden.ProcessSpec{
		Privileged: privilegedProcess,
		Path:       "/usr/bin/hellofs",
		Args:       []string{mountpoint},
	}, garden.ProcessIO{Stdout: GinkgoWriter, Stderr: GinkgoWriter})
	Ω(err).ShouldNot(HaveOccurred())
	Ω(process.Wait()).Should(Equal(0), "Failed to mount hello filesystem.")

	stdout := gbytes.NewBuffer()
	process, err = container.Run(garden.ProcessSpec{
		Privileged: privilegedProcess,
		Path:       "cat",
		Args:       []string{filepath.Join(mountpoint, "hello")},
	}, garden.ProcessIO{Stdout: stdout, Stderr: GinkgoWriter})
	Ω(err).ShouldNot(HaveOccurred())
	Ω(process.Wait()).Should(Equal(0), "Failed to find hello file.")
	Ω(stdout).Should(gbytes.Say("Hello World!"))

	process, err = container.Run(garden.ProcessSpec{
		Privileged: privilegedProcess,
		Path:       "fusermount",
		Args:       []string{"-u", mountpoint},
	}, garden.ProcessIO{Stdout: GinkgoWriter, Stderr: GinkgoWriter})
	Ω(err).ShouldNot(HaveOccurred())
	Ω(process.Wait()).Should(Equal(0), "Failed to unmount user filesystem.")

	stdout2 := gbytes.NewBuffer()
	process, err = container.Run(garden.ProcessSpec{
		Privileged: privilegedProcess,
		Path:       "ls",
		Args:       []string{mountpoint},
	}, garden.ProcessIO{Stdout: stdout2, Stderr: GinkgoWriter})
	Ω(err).ShouldNot(HaveOccurred())
	Ω(process.Wait()).Should(Equal(0))
	Ω(stdout2).ShouldNot(gbytes.Say("hello"), "Fuse filesystem appears still to be visible after being unmounted.")
}
