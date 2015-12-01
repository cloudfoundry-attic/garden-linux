package system_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"sync"
	"syscall"
	//"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

// const rootDir = "/tmp/aufs/tmpfs"
const rootDir = "/tmp"
const withAufs = true

var _ = Describe("Weirdness", func() {

	FIt("doesn't cause the problem if we try putting the system under load and then unmounting", func() {

		// build the pivotrooter
		simpleMounterBin, err := gexec.Build("github.com/cloudfoundry-incubator/garden-linux/containerizer/system/simple_pivotrooter", "-race")
		Expect(err).NotTo(HaveOccurred())

		var wg sync.WaitGroup
		go bindMountUnmountRemoveLots("Alpha:", &wg, simpleMounterBin)
		wg.Add(1)
		go bindMountUnmountRemoveLots("Beta :", &wg, simpleMounterBin)
		wg.Add(1)
		wg.Wait()
	})
})

func bindMountUnmountRemoveLots(msg string, wg *sync.WaitGroup, binary string) {
	defer GinkgoRecover()
	defer wg.Done()
	for i := 0; i < 10; i++ {
		fmt.Fprintf(os.Stderr, "%s: iteration %d\n", msg, i)
		bindMountUnmountRemove(msg, binary)
	}
}

func bindMountUnmountRemove(msg string, binary string) {
	targetBindMountDir, err := ioutil.TempDir(rootDir, "target")
	Expect(err).ToNot(HaveOccurred())
	fmt.Fprintf(os.Stderr, "%s:  made targetBindMountDir\n", msg)

	sourceBindMountDir, err := ioutil.TempDir(rootDir, "source")
	Expect(err).ToNot(HaveOccurred())
	fmt.Fprintf(os.Stderr, "%s:  made sourceBindMountDir\n", msg)

	if withAufs {
		roDir, err := ioutil.TempDir(rootDir, "ro")
		Expect(err).ToNot(HaveOccurred())

		rwDir, err := ioutil.TempDir(rootDir, "rw")
		Expect(err).ToNot(HaveOccurred())

		Expect(syscall.Mount("none", sourceBindMountDir, "aufs", 0, fmt.Sprintf("br:%s=rw:%s=ro", rwDir, roDir)) ).To(Succeed())
	}

	Expect(syscall.Mount(sourceBindMountDir, targetBindMountDir, "", uintptr(syscall.MS_BIND), "")).To(Succeed())
	//Expect(syscall.Mount(sourceBindMountDir, targetBindMountDir, "", uintptr(syscall.MS_BIND|syscall.MS_PRIVATE), "")).To(Succeed())
	fmt.Fprintf(os.Stderr, "%s:  mounted \n", msg)

	// rprivingcmd := exec.Command("mount", "--make-rprivate", "/")
	// err = rprivingcmd.Run()
	// Expect(err).NotTo(HaveOccurred())

	fmt.Fprintf(os.Stderr, "%s: rprived the /\n", msg)

	// Call the pivotrooter with targetBindMountDir and msg
	flags := syscall.CLONE_NEWNS
	cmd := exec.Command(binary, targetBindMountDir, msg	)
	cmd.Stdout = GinkgoWriter
	cmd.Stderr = GinkgoWriter
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: uintptr(flags),
	}
	err = cmd.Run()
	Expect(err).NotTo(HaveOccurred())

	fmt.Fprintf(os.Stderr, "%s done running command\n", msg)

	// If you comment out the following line, the VM doesn't die.
	Expect(syscall.Unmount(targetBindMountDir, 0)).To(Succeed())
	fmt.Fprintf(os.Stderr, "%s:  unmounted\n", msg)
//	time.Sleep(500 * time.Millisecond)
	// If you don't comment out the previous line, then the next line causes the VM to die :D
	Expect(os.RemoveAll(targetBindMountDir)).To(Succeed())
	fmt.Fprintf(os.Stderr, "%s:  removed targetBindMountDir\n", msg)
	Expect(os.RemoveAll(sourceBindMountDir)).To(Succeed())
	fmt.Fprintf(os.Stderr, "%s:  removed sourceBindMountDir\n", msg)

}
