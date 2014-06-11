// +build linux

package wshd_test

import (
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"syscall"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gbytes"
	. "github.com/onsi/gomega/gexec"
)

var _ = Describe("Running wshd", func() {
	wshd := "../../linux_backend/skeleton/bin/wshd"

	wsh := "../../linux_backend/skeleton/bin/wsh"

	shmTest, err := Build("github.com/cloudfoundry-incubator/warden-linux/integration/wshd/shm_test")
	if err != nil {
		panic(err)
	}

	var socketPath string
	var containerPath string

	var binDir string
	var libDir string
	var runDir string
	var mntDir string

	BeforeEach(func() {
		var err error

		containerPath, err = ioutil.TempDir(os.TempDir(), "wshd-test-container")
		Expect(err).ToNot(HaveOccurred())

		binDir = path.Join(containerPath, "bin")
		libDir = path.Join(containerPath, "lib")
		runDir = path.Join(containerPath, "run")
		mntDir = path.Join(containerPath, "mnt")

		os.Mkdir(binDir, 0755)
		os.Mkdir(libDir, 0755)
		os.Mkdir(runDir, 0755)

		err = copyFile(wshd, path.Join(binDir, "wshd"))
		Expect(err).ToNot(HaveOccurred())

		ioutil.WriteFile(path.Join(libDir, "hook-parent-before-clone.sh"), []byte(`#!/bin/bash

set -o nounset
set -o errexit
shopt -s nullglob

cd $(dirname $0)/../

cp bin/wshd mnt/sbin/wshd
chmod 700 mnt/sbin/wshd
`), 0755)

		ioutil.WriteFile(path.Join(libDir, "hook-parent-after-clone.sh"), []byte(`#!/bin/bash
set -o nounset
set -o errexit
shopt -s nullglob

cd $(dirname $0)/../

echo $PID > ./run/wshd.pid
`), 0755)

		ioutil.WriteFile(path.Join(libDir, "hook-child-before-pivot.sh"), []byte(`#!/bin/bash
`), 0755)

		ioutil.WriteFile(path.Join(libDir, "hook-child-after-pivot.sh"), []byte(`#!/bin/bash

set -o nounset
set -o errexit
shopt -s nullglob

cd $(dirname $0)/../

mkdir -p /proc
mount -t proc none /proc

useradd -mU -u 10000 -s /bin/bash vcap
`), 0755)

		ioutil.WriteFile(path.Join(libDir, "set-up-root.sh"), []byte(`#!/bin/bash

set -o nounset
set -o errexit
shopt -s nullglob

rootfs_path=$1

function overlay_directory_in_rootfs() {
  # Skip if exists
  if [ ! -d tmp/rootfs/$1 ]
  then
    if [ -d mnt/$1 ]
    then
      cp -r mnt/$1 tmp/rootfs/
    else
      mkdir -p tmp/rootfs/$1
    fi
  fi

  mount -n --bind tmp/rootfs/$1 mnt/$1
  mount -n --bind -o remount,$2 tmp/rootfs/$1 mnt/$1
}

function setup_fs() {
  mkdir -p tmp/rootfs mnt

  mkdir -p $rootfs_path/proc

  mount -n --bind $rootfs_path mnt
  mount -n --bind -o remount,ro $rootfs_path mnt

  overlay_directory_in_rootfs /dev rw
  overlay_directory_in_rootfs /etc rw
  overlay_directory_in_rootfs /home rw
  overlay_directory_in_rootfs /sbin rw
  overlay_directory_in_rootfs /var rw

  mkdir -p tmp/rootfs/tmp
  chmod 777 tmp/rootfs/tmp
  overlay_directory_in_rootfs /tmp rw
}

setup_fs
`), 0755)

		setUpRoot := exec.Command(path.Join(libDir, "set-up-root.sh"), os.Getenv("WARDEN_TEST_ROOTFS"))
		setUpRoot.Dir = containerPath

		setUpRootSession, err := Start(setUpRoot, GinkgoWriter, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())
		Eventually(setUpRootSession, 5.0).Should(Exit(0))
	})

	JustBeforeEach(func() {
		wshdCommand := exec.Command(
			wshd,
			"--run", runDir,
			"--lib", libDir,
			"--root", mntDir,
			"--title", "test wshd",
		)

		socketPath = path.Join(runDir, "wshd.sock")

		wshdSession, err := Start(wshdCommand, GinkgoWriter, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())

		Eventually(wshdSession, 30).Should(Exit(0))

		Eventually(ErrorDialingUnix(socketPath)).ShouldNot(HaveOccurred())
	})

	AfterEach(func() {
		wshdPidfile, err := os.Open(path.Join(containerPath, "run", "wshd.pid"))
		Ω(err).ShouldNot(HaveOccurred())

		var wshdPid int
		_, err = fmt.Fscanf(wshdPidfile, "%d", &wshdPid)
		Ω(err).ShouldNot(HaveOccurred())

		proc, err := os.FindProcess(wshdPid)
		Ω(err).ShouldNot(HaveOccurred())

		err = proc.Kill()
		Ω(err).ShouldNot(HaveOccurred())

		for _, submount := range []string{"dev", "etc", "home", "sbin", "var", "tmp"} {
			mountPoint := path.Join(containerPath, "mnt", submount)

			err := syscall.Unmount(mountPoint, 0)
			Ω(err).ShouldNot(HaveOccurred())
		}

		err = syscall.Unmount(path.Join(containerPath, "mnt"), 0)
		Ω(err).ShouldNot(HaveOccurred())

		Eventually(func() error {
			return os.RemoveAll(containerPath)
		}, 10).ShouldNot(HaveOccurred())
	})

	It("starts the daemon as a session leader with process isolation and the given title", func() {
		ps := exec.Command(wsh, "--socket", socketPath, "/bin/ps", "-o", "pid,command")

		psSession, err := Start(ps, GinkgoWriter, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())

		Eventually(psSession).Should(Say(`  PID COMMAND
    1 test wshd\s+
   \d+ /bin/ps -o pid,command
`))

		Eventually(psSession).Should(Exit(0))

		Expect(psSession).ShouldNot(Say("."))
	})

	It("starts the daemon with mount space isolation", func() {
		mkdir := exec.Command(wsh, "--socket", socketPath, "/bin/mkdir", "/home/vcap/lawn")
		mkdirSession, err := Start(mkdir, GinkgoWriter, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())
		Eventually(mkdirSession).Should(Exit(0))

		mkdir = exec.Command(wsh, "--socket", socketPath, "/bin/mkdir", "/home/vcap/gnome")
		mkdirSession, err = Start(mkdir, GinkgoWriter, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())
		Eventually(mkdirSession).Should(Exit(0))

		mount := exec.Command(wsh, "--socket", socketPath, "/bin/mount", "--bind", "/home/vcap/lawn", "/home/vcap/gnome")
		mountSession, err := Start(mount, GinkgoWriter, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())
		Eventually(mountSession).Should(Exit(0))

		cat := exec.Command("/bin/cat", "/proc/mounts")
		catSession, err := Start(cat, GinkgoWriter, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())
		Expect(catSession).ToNot(Say("gnome"))
		Eventually(catSession).Should(Exit(0))
	})

	It("places the daemon in each cgroup subsystem", func() {
		cat := exec.Command(wsh, "--socket", socketPath, "bash", "-c", "cat /proc/$$/cgroup")
		catSession, err := Start(cat, GinkgoWriter, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())
		Eventually(catSession).Should(Exit(0))
		Expect(catSession.Out.Contents()).To(MatchRegexp(`\bcpu\b`))
		Expect(catSession.Out.Contents()).To(MatchRegexp(`\bcpuacct\b`))
		Expect(catSession.Out.Contents()).To(MatchRegexp(`\bcpuset\b`))
		Expect(catSession.Out.Contents()).To(MatchRegexp(`\bdevices\b`))
		Expect(catSession.Out.Contents()).To(MatchRegexp(`\bmemory\b`))
	})

	It("starts the daemon with network namespace isolation", func() {
		ifconfig := exec.Command(wsh, "--socket", socketPath, "/sbin/ifconfig", "lo:0", "1.2.3.4", "up")
		ifconfigSession, err := Start(ifconfig, GinkgoWriter, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())
		Eventually(ifconfigSession).Should(Exit(0))

		localIfconfig := exec.Command("ifconfig")
		localIfconfigSession, err := Start(localIfconfig, GinkgoWriter, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())
		Expect(localIfconfigSession).ToNot(Say("lo:0"))
		Eventually(localIfconfigSession).Should(Exit(0))
	})

	It("starts the daemon with a new IPC namespace", func() {
		err = copyFile(shmTest, path.Join(mntDir, "sbin", "shmtest"))
		Expect(err).ToNot(HaveOccurred())

		localSHM := exec.Command(shmTest)
		createLocal, err := Start(
			localSHM,
			GinkgoWriter,
			GinkgoWriter,
		)
		Expect(err).ToNot(HaveOccurred())

		Eventually(createLocal).Should(Say("ok"))

		createRemote, err := Start(
			exec.Command(wsh, "--socket", socketPath, "/sbin/shmtest", "create"),
			GinkgoWriter,
			GinkgoWriter,
		)
		Expect(err).ToNot(HaveOccurred())
		Eventually(createRemote).Should(Say("ok"))

		localSHM.Process.Signal(syscall.SIGUSR2)

		Eventually(createLocal).Should(Exit(0))
	})

	It("starts the daemon with a new UTS namespace", func() {
		hostname := exec.Command(wsh, "--socket", socketPath, "/bin/hostname", "newhostname")
		hostnameSession, err := Start(hostname, GinkgoWriter, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())

		Eventually(hostnameSession).Should(Exit(0))

		localHostname := exec.Command("hostname")
		localHostnameSession, err := Start(localHostname, GinkgoWriter, GinkgoWriter)
		Expect(localHostnameSession).ToNot(Say("newhostname"))
	})

	It("does not leak any shared memory to the child", func() {
		createRemote, err := Start(
			exec.Command(wsh, "--socket", socketPath, "ipcs"),
			GinkgoWriter,
			GinkgoWriter,
		)
		Expect(err).ToNot(HaveOccurred())
		Expect(createRemote).ToNot(Say("deadbeef"))
	})

	It("unmounts /tmp/warden-host* in the child", func() {
		cat := exec.Command(wsh, "--socket", socketPath, "/bin/cat", "/proc/mounts")

		catSession, err := Start(cat, GinkgoWriter, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())

		Expect(catSession).ToNot(Say(" /tmp/warden-host"))
		Eventually(catSession).Should(Exit(0))
	})

	Context("when mount points on the host are deleted", func() {
		BeforeEach(func() {
			tmpdir, err := ioutil.TempDir("", "wshd-bogus-mount")
			Expect(err).ToNot(HaveOccurred())

			fooDir := filepath.Join(tmpdir, "foo")
			barDir := filepath.Join(tmpdir, "bar")

			err = os.MkdirAll(fooDir, 0755)
			Expect(err).ToNot(HaveOccurred())

			err = os.MkdirAll(barDir, 0755)
			Expect(err).ToNot(HaveOccurred())

			mount := exec.Command("mount", "--bind", fooDir, barDir)
			mountSession, err := Start(mount, GinkgoWriter, GinkgoWriter)
			Expect(err).ToNot(HaveOccurred())
			Eventually(mountSession).Should(Exit(0))

			err = os.RemoveAll(fooDir)
			Expect(err).ToNot(HaveOccurred())

			cat := exec.Command("/bin/cat", "/proc/mounts")
			catSession, err := Start(cat, GinkgoWriter, GinkgoWriter)
			Expect(err).ToNot(HaveOccurred())
			Eventually(catSession).Should(Say("(deleted)"))
			Eventually(catSession).Should(Exit(0))
		})

		It("unmounts the un-mangled mount point name", func() {
			cat := exec.Command(wsh, "--socket", socketPath, "/bin/cat", "/proc/mounts")

			catSession, err := Start(cat, GinkgoWriter, GinkgoWriter)
			Expect(err).ToNot(HaveOccurred())

			Expect(catSession).ToNot(Say("(deleted)"))
			Eventually(catSession).Should(Exit(0))
		})
	})

	Context("when running a command as a user", func() {
		It("executes with setuid and setgid", func() {
			bash := exec.Command(wsh, "--socket", socketPath, "--user", "vcap", "/bin/bash", "-c", "id -u; id -g")

			bashSession, err := Start(bash, GinkgoWriter, GinkgoWriter)
			Expect(err).ToNot(HaveOccurred())

			Eventually(bashSession).Should(Say("^10000\n"))
			Eventually(bashSession).Should(Say("^10000\n"))
			Eventually(bashSession).Should(Exit(0))
		})

		It("sets $HOME, $USER, and $PATH", func() {
			bash := exec.Command(wsh, "--socket", socketPath, "--user", "vcap", "/bin/bash", "-c", "env | sort")

			bashSession, err := Start(bash, GinkgoWriter, GinkgoWriter)
			Expect(err).ToNot(HaveOccurred())

			Eventually(bashSession).Should(Say("HOME=/home/vcap\n"))
			Eventually(bashSession).Should(Say("PATH=/usr/local/bin:/usr/bin:/bin\n"))
			Eventually(bashSession).Should(Say("USER=vcap\n"))
			Eventually(bashSession).Should(Exit(0))
		})

		It("executes in their home directory", func() {
			pwd := exec.Command(wsh, "--socket", socketPath, "--user", "vcap", "/bin/pwd")

			pwdSession, err := Start(pwd, GinkgoWriter, GinkgoWriter)
			Expect(err).ToNot(HaveOccurred())

			Eventually(pwdSession).Should(Say("/home/vcap\n"))
			Eventually(pwdSession).Should(Exit(0))
		})
	})

	Context("when running a command as root", func() {
		It("executes with setuid and setgid", func() {
			bash := exec.Command(wsh, "--socket", socketPath, "--user", "root", "/bin/bash", "-c", "id -u; id -g")

			bashSession, err := Start(bash, GinkgoWriter, GinkgoWriter)
			Expect(err).ToNot(HaveOccurred())

			Eventually(bashSession).Should(Say("^0\n"))
			Eventually(bashSession).Should(Say("^0\n"))
			Eventually(bashSession).Should(Exit(0))
		})

		It("sets $HOME, $USER, and a $PATH with sbin dirs", func() {
			bash := exec.Command(wsh, "--socket", socketPath, "--user", "root", "/bin/bash", "-c", "env | sort")

			bashSession, err := Start(bash, GinkgoWriter, GinkgoWriter)
			Expect(err).ToNot(HaveOccurred())

			Eventually(bashSession).Should(Say("HOME=/root\n"))
			Eventually(bashSession).Should(Say("PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin\n"))
			Eventually(bashSession).Should(Say("USER=root\n"))
			Eventually(bashSession).Should(Exit(0))
		})

		It("executes in their home directory", func() {
			pwd := exec.Command(wsh, "--socket", socketPath, "--user", "root", "/bin/pwd")

			pwdSession, err := Start(pwd, GinkgoWriter, GinkgoWriter)
			Expect(err).ToNot(HaveOccurred())

			Eventually(pwdSession).Should(Say("/root\n"))
			Eventually(pwdSession).Should(Exit(0))
		})
	})

	Context("when piping stdin", func() {
		It("terminates when the input stream terminates", func() {
			bash := exec.Command(wsh, "--socket", socketPath, "/bin/bash")

			stdin, err := bash.StdinPipe()
			Expect(err).ToNot(HaveOccurred())

			bashSession, err := Start(bash, GinkgoWriter, GinkgoWriter)
			Expect(err).ToNot(HaveOccurred())

			stdin.Write([]byte("echo hello"))
			stdin.Close()

			Eventually(bashSession).Should(Say("hello\n"))
			Eventually(bashSession).Should(Exit(0))
		})
	})

	Context("when in rsh compatibility mode", func() {
		It("respects -l, discards -t [X], -46dn, skips the host, and runs the command", func() {
			pwd := exec.Command(
				wsh,
				"--socket", socketPath,
				"--user", "root",
				"--rsh",
				"-l", "vcap",
				"-t", "1",
				"-4",
				"-6",
				"-d",
				"-n",
				"somehost",
				"/bin/pwd",
			)

			pwdSession, err := Start(pwd, GinkgoWriter, GinkgoWriter)
			Expect(err).ToNot(HaveOccurred())

			Eventually(pwdSession).Should(Say("/home/vcap\n"))
			Eventually(pwdSession).Should(Exit(0))
		})

		It("doesn't cause rsh-like flags to be consumed", func() {
			cmd := exec.Command(
				wsh,
				"--socket", socketPath,
				"--user", "root",
				"/bin/echo",
				"-l", "vcap",
				"-t", "1",
				"-4",
				"-6",
				"-d",
				"-n",
				"somehost",
			)

			cmdSession, err := Start(cmd, GinkgoWriter, GinkgoWriter)
			Expect(err).ToNot(HaveOccurred())

			Eventually(cmdSession).Should(Say("-l vcap -t 1 -4 -6 -d -n somehost\n"))
			Eventually(cmdSession).Should(Exit(0))
		})

		It("can be used to rsync files", func() {
			cmd := exec.Command(
				"rsync",
				"-e",
				wsh+" --socket "+socketPath+" --rsh",
				"-r",
				"-p",
				"--links",
				wsh, // send wsh binary
				"vcap@container:wsh",
			)

			cmdSession, err := Start(cmd, GinkgoWriter, GinkgoWriter)
			Expect(err).ToNot(HaveOccurred())

			Eventually(cmdSession).Should(Exit(0))
		})
	})
})

func copyFile(src, dst string) error {
	s, err := os.Open(src)
	if err != nil {
		return err
	}

	defer s.Close()

	d, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE, 0755)
	if err != nil {
		return err
	}

	_, err = io.Copy(d, s)
	if err != nil {
		d.Close()
		return err
	}

	return d.Close()
}

func ErrorDialingUnix(socketPath string) func() error {
	return func() error {
		conn, err := net.Dial("unix", socketPath)
		if err == nil {
			conn.Close()
		}

		return err
	}
}
