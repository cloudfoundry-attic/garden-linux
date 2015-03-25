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

	shmTest, err := Build("github.com/cloudfoundry-incubator/garden-linux/old/integration/wshd/shm_test")
	if err != nil {
		panic(err)
	}

	var socketPath string
	var containerPath string

	var binDir string
	var libDir string
	var runDir string
	var mntDir string

	var userNs string

	var beforeWshd func()

	BeforeEach(func() {
		var err error

		containerPath, err = ioutil.TempDir(os.TempDir(), "wshd-test-container")
		Ω(err).ShouldNot(HaveOccurred())

		userNs = "disabled"
		beforeWshd = func() {}

		binDir = path.Join(containerPath, "bin")
		libDir = path.Join(containerPath, "lib")
		runDir = path.Join(containerPath, "run")
		mntDir = path.Join(containerPath, "mnt")

		os.Mkdir(binDir, 0755)
		os.Mkdir(libDir, 0755)
		os.Mkdir(runDir, 0755)

		err = copyFile(wshd, path.Join(binDir, "wshd"))
		Ω(err).ShouldNot(HaveOccurred())

		hookPath, err := Build("github.com/cloudfoundry-incubator/garden-linux/old/integration/wshd/fake_hook")
		Ω(err).ShouldNot(HaveOccurred())
		err = copyFile(hookPath, path.Join(libDir, "hook"))
		Ω(err).ShouldNot(HaveOccurred())

		ioutil.WriteFile(path.Join(libDir, "hook-parent-before-clone.sh"), []byte(`#!/bin/sh

set -o nounset
set -o errexit

cd $(dirname $0)/../

cp bin/wshd mnt/sbin/wshd
chmod 700 mnt/sbin/wshd
`), 0755)

		ioutil.WriteFile(path.Join(libDir, "hook-parent-after-clone.sh"), []byte(`#!/bin/sh
set -o nounset
set -o errexit

cd $(dirname $0)/../

cat > /proc/$PID/uid_map 2> /dev/null <<EOF || true
0 0 1
10000 10000 1
EOF

cat > /proc/$PID/gid_map 2> /dev/null <<EOF || true
0 0 1
10000 10000 1
EOF

echo $PID > ./run/wshd.pid
`), 0755)

		ioutil.WriteFile(path.Join(libDir, "hook-child-after-pivot.sh"), []byte(`#!/bin/sh

set -o nounset
set -o errexit

cd $(dirname $0)/../

mkdir -p /proc
mount -t proc none /proc

adduser -u 10000 -g 10000 -s /bin/sh -D vcap
`), 0755)

		ioutil.WriteFile(path.Join(libDir, "set-up-root.sh"), []byte(`#!/bin/bash

set -o nounset
set -o errexit

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

  # test asserts that wshd changes this to 0777
  chmod 755 tmp/rootfs/tmp

  overlay_directory_in_rootfs /tmp rw
}

setup_fs
`), 0755)

		setUpRoot := exec.Command(path.Join(libDir, "set-up-root.sh"), os.Getenv("GARDEN_TEST_ROOTFS"))
		setUpRoot.Dir = containerPath

		setUpRootSession, err := Start(setUpRoot, GinkgoWriter, GinkgoWriter)
		Ω(err).ShouldNot(HaveOccurred())
		Eventually(setUpRootSession, 5.0).Should(Exit(0))
	})

	JustBeforeEach(func() {

		beforeWshd()

		wshdCommand := exec.Command(
			wshd,
			"--run", runDir,
			"--lib", libDir,
			"--root", mntDir,
			"--title", "test wshd",
			"--userns", userNs,
		)

		socketPath = path.Join(runDir, "wshd.sock")

		wshdSession, err := Start(wshdCommand, GinkgoWriter, GinkgoWriter)
		Ω(err).ShouldNot(HaveOccurred())

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
		ps := exec.Command(wsh, "--socket", socketPath, "/bin/ps", "-o", "pid,comm")

		psSession, err := Start(ps, GinkgoWriter, GinkgoWriter)
		Ω(err).ShouldNot(HaveOccurred())

		Eventually(psSession).Should(Say(`\s+1\s+wshd`))
		Eventually(psSession).Should(Exit(0))
	})

	It("starts the daemon with mount space isolation", func() {
		mkdir := exec.Command(wsh, "--socket", socketPath, "/bin/mkdir", "/home/vcap/lawn")
		mkdirSession, err := Start(mkdir, GinkgoWriter, GinkgoWriter)
		Ω(err).ShouldNot(HaveOccurred())
		Eventually(mkdirSession).Should(Exit(0))

		mkdir = exec.Command(wsh, "--socket", socketPath, "/bin/mkdir", "/home/vcap/gnome")
		mkdirSession, err = Start(mkdir, GinkgoWriter, GinkgoWriter)
		Ω(err).ShouldNot(HaveOccurred())
		Eventually(mkdirSession).Should(Exit(0))

		mount := exec.Command(wsh, "--socket", socketPath, "/bin/mount", "--bind", "/home/vcap/lawn", "/home/vcap/gnome")
		mountSession, err := Start(mount, GinkgoWriter, GinkgoWriter)
		Ω(err).ShouldNot(HaveOccurred())
		Eventually(mountSession).Should(Exit(0))

		cat := exec.Command("/bin/cat", "/proc/mounts")
		catSession, err := Start(cat, GinkgoWriter, GinkgoWriter)
		Ω(err).ShouldNot(HaveOccurred())
		Eventually(catSession).Should(Exit(0))
		Ω(catSession).ShouldNot(Say("gnome"))
	})

	It("places the daemon in each cgroup subsystem", func() {
		cat := exec.Command(wsh, "--socket", socketPath, "sh", "-c", "cat /proc/$$/cgroup")
		catSession, err := Start(cat, GinkgoWriter, GinkgoWriter)
		Ω(err).ShouldNot(HaveOccurred())
		Eventually(catSession).Should(Exit(0))
		Ω(catSession.Out.Contents()).Should(MatchRegexp(`\bcpu\b`))
		Ω(catSession.Out.Contents()).Should(MatchRegexp(`\bcpuacct\b`))
		Ω(catSession.Out.Contents()).Should(MatchRegexp(`\bcpuset\b`))
		Ω(catSession.Out.Contents()).Should(MatchRegexp(`\bdevices\b`))
		Ω(catSession.Out.Contents()).Should(MatchRegexp(`\bmemory\b`))
	})

	It("starts the daemon with network namespace isolation", func() {
		ifconfig := exec.Command(wsh, "--socket", socketPath, "/sbin/ifconfig", "lo:0", "1.2.3.4", "up")
		ifconfigSession, err := Start(ifconfig, GinkgoWriter, GinkgoWriter)
		Ω(err).ShouldNot(HaveOccurred())
		Eventually(ifconfigSession).Should(Exit(0))

		localIfconfig := exec.Command("ifconfig")
		localIfconfigSession, err := Start(localIfconfig, GinkgoWriter, GinkgoWriter)
		Ω(err).ShouldNot(HaveOccurred())
		Eventually(localIfconfigSession).Should(Exit(0))
		Ω(localIfconfigSession).ShouldNot(Say("lo:0"))
	})

	It("starts the daemon with a new IPC namespace", func() {
		err = copyFile(shmTest, path.Join(mntDir, "sbin", "shmtest"))
		Ω(err).ShouldNot(HaveOccurred())

		localSHM := exec.Command(shmTest)
		createLocal, err := Start(
			localSHM,
			GinkgoWriter,
			GinkgoWriter,
		)
		Ω(err).ShouldNot(HaveOccurred())

		Eventually(createLocal).Should(Say("ok"))

		createRemote, err := Start(
			exec.Command(wsh, "--socket", socketPath, "/sbin/shmtest", "create"),
			GinkgoWriter,
			GinkgoWriter,
		)
		Ω(err).ShouldNot(HaveOccurred())
		Eventually(createRemote).Should(Say("ok"))

		localSHM.Process.Signal(syscall.SIGUSR2)

		Eventually(createLocal).Should(Exit(0))
	})

	It("starts the daemon with a new UTS namespace", func() {
		hostname := exec.Command(wsh, "--socket", socketPath, "/bin/hostname", "newhostname")
		hostnameSession, err := Start(hostname, GinkgoWriter, GinkgoWriter)
		Ω(err).ShouldNot(HaveOccurred())
		Eventually(hostnameSession).Should(Exit(0))

		localHostname := exec.Command("hostname")
		localHostnameSession, err := Start(localHostname, GinkgoWriter, GinkgoWriter)
		Eventually(localHostnameSession).Should(Exit(0))
		Ω(localHostnameSession).ShouldNot(Say("newhostname"))
	})

	It("does not leak any shared memory to the child", func() {
		ipcs, err := Start(
			exec.Command(wsh, "--socket", socketPath, "ipcs"),
			GinkgoWriter,
			GinkgoWriter,
		)
		Ω(err).ShouldNot(HaveOccurred())
		Eventually(ipcs).Should(Exit(0))
		Ω(ipcs).ShouldNot(Say("deadbeef"))
	})

	It("ensures /tmp is world-writable", func() {
		ls, err := Start(
			exec.Command(wsh, "--socket", socketPath, "ls", "-al", "/tmp"),
			GinkgoWriter,
			GinkgoWriter,
		)
		Ω(err).ShouldNot(HaveOccurred())
		Eventually(ls).Should(Exit(0))

		Ω(ls).Should(Say(`drwxrwxrwt`))
	})

	It("unmounts /tmp/garden-host* in the child", func() {
		cat := exec.Command(wsh, "--socket", socketPath, "/bin/cat", "/proc/mounts")

		catSession, err := Start(cat, GinkgoWriter, GinkgoWriter)
		Ω(err).ShouldNot(HaveOccurred())

		Eventually(catSession).Should(Exit(0))
		Ω(catSession).ShouldNot(Say(" /tmp/garden-host"))
	})

	Context("when mount points on the host are deleted", func() {
		BeforeEach(func() {
			tmpdir, err := ioutil.TempDir("", "wshd-bogus-mount")
			Ω(err).ShouldNot(HaveOccurred())

			fooDir := filepath.Join(tmpdir, "foo")
			barDir := filepath.Join(tmpdir, "bar")

			err = os.MkdirAll(fooDir, 0755)
			Ω(err).ShouldNot(HaveOccurred())

			err = os.MkdirAll(barDir, 0755)
			Ω(err).ShouldNot(HaveOccurred())

			mount := exec.Command("mount", "--bind", fooDir, barDir)
			mountSession, err := Start(mount, GinkgoWriter, GinkgoWriter)
			Ω(err).ShouldNot(HaveOccurred())
			Eventually(mountSession).Should(Exit(0))

			err = os.RemoveAll(fooDir)
			Ω(err).ShouldNot(HaveOccurred())

			cat := exec.Command("/bin/cat", "/proc/mounts")
			catSession, err := Start(cat, GinkgoWriter, GinkgoWriter)
			Ω(err).ShouldNot(HaveOccurred())
			Eventually(catSession).Should(Say("(deleted)"))
			Eventually(catSession).Should(Exit(0))
		})

		It("unmounts the un-mangled mount point name", func() {
			cat := exec.Command(wsh, "--socket", socketPath, "/bin/cat", "/proc/mounts")

			catSession, err := Start(cat, GinkgoWriter, GinkgoWriter)
			Ω(err).ShouldNot(HaveOccurred())

			Eventually(catSession).Should(Exit(0))
			Ω(catSession).ShouldNot(Say("(deleted)"))
		})
	})

	Context("when running a command in a working dir", func() {
		It("executes with setuid and setgid", func() {
			pwd := exec.Command(wsh, "--socket", socketPath, "--dir", "/usr", "pwd")

			pwdSession, err := Start(pwd, GinkgoWriter, GinkgoWriter)
			Ω(err).ShouldNot(HaveOccurred())

			Eventually(pwdSession).Should(Say("^/usr\n"))
			Eventually(pwdSession).Should(Exit(0))
		})
	})

	Context("when running without specifying a --pidfile", func() {
		It("should exit cleanly with the correct status", func() {
			pwd := exec.Command(wsh, "--socket", socketPath, "--dir", "/usr", "/bin/sh", "-c", "exit 3")

			stdout := NewBuffer()
			stderr := NewBuffer()
			pwdSession, err := Start(pwd, io.MultiWriter(stdout, GinkgoWriter), io.MultiWriter(stderr, GinkgoWriter))
			Ω(err).ShouldNot(HaveOccurred())

			Eventually(pwdSession).Should(Exit(3))
			Ω(string(stderr.Contents())).Should(Equal(""))
			Ω(string(stdout.Contents())).Should(Equal(""))
		})
	})

	It("allows children to receive SIGCHLD when grandchildren die", func() {
		trap := exec.Command(wsh, "--socket", socketPath, "--dir", "/usr", "/bin/sh", "-c", "trap 'echo caught sigchld' SIGCHLD; $(ls / >/dev/null 2>&1); sleep 5;")

		trapSession, err := Start(trap, GinkgoWriter, GinkgoWriter)
		Ω(err).ShouldNot(HaveOccurred())

		Eventually(trapSession).Should(Say("caught sigchld"))
	})

	Context("when running a command as a user", func() {
		It("executes with setuid and setgid", func() {
			sh := exec.Command(wsh, "--socket", socketPath, "--user", "vcap", "/bin/sh", "-c", "id -u; id -g")

			shSession, err := Start(sh, GinkgoWriter, GinkgoWriter)
			Ω(err).ShouldNot(HaveOccurred())

			Eventually(shSession).Should(Say("^10000\n"))
			Eventually(shSession).Should(Say("^10000\n"))
			Eventually(shSession).Should(Exit(0))
		})

		It("sets $HOME, $USER, and $PATH", func() {
			sh := exec.Command(wsh, "--socket", socketPath, "--user", "vcap", "/bin/sh", "-c", "env | sort")

			shSession, err := Start(sh, GinkgoWriter, GinkgoWriter)
			Ω(err).ShouldNot(HaveOccurred())

			Eventually(shSession).Should(Say("HOME=/home/vcap\n"))
			Eventually(shSession).Should(Say("PATH=/usr/local/bin:/usr/bin:/bin\n"))
			Eventually(shSession).Should(Say("USER=vcap\n"))
			Eventually(shSession).Should(Exit(0))
		})

		It("executes in their home directory", func() {
			pwd := exec.Command(wsh, "--socket", socketPath, "--user", "vcap", "/bin/pwd")

			pwdSession, err := Start(pwd, GinkgoWriter, GinkgoWriter)
			Ω(err).ShouldNot(HaveOccurred())

			Eventually(pwdSession).Should(Say("/home/vcap\n"))
			Eventually(pwdSession).Should(Exit(0))
		})

		It("sets the specified environment variables", func() {
			pwd := exec.Command(wsh,
				"--socket", socketPath,
				"--user", "vcap",
				"--env", "VAR1=VALUE1",
				"--env", "VAR2=VALUE2",
				"sh", "-c", "env | sort",
			)

			session, err := Start(pwd, GinkgoWriter, GinkgoWriter)
			Ω(err).ShouldNot(HaveOccurred())

			Eventually(session).Should(Say("VAR1=VALUE1\n"))
			Eventually(session).Should(Say("VAR2=VALUE2\n"))
		})

		It("searches a sanitized path not including sbin for the executable", func() {
			ls := exec.Command(wsh,
				"--socket", socketPath,
				"--user", "vcap",
				"ls",
			)

			session, err := Start(ls, GinkgoWriter, GinkgoWriter)
			Ω(err).ShouldNot(HaveOccurred())

			Eventually(session).Should(Exit(0))

			onlyInSbin := exec.Command(wsh,
				"--socket", socketPath,
				"--user", "vcap",
				"ifconfig",
			)

			session, err = Start(onlyInSbin, GinkgoWriter, GinkgoWriter)
			Ω(err).ShouldNot(HaveOccurred())

			Eventually(session).Should(Exit(255))
		})

		It("saves the child's pid in a pidfile and cleans the pidfile up after the process exits", func() {
			tmp, err := ioutil.TempDir("", "wshdchildpid")
			Ω(err).ShouldNot(HaveOccurred())

			pwd := exec.Command(wsh,
				"--socket", socketPath,
				"--user", "vcap",
				"--pidfile", filepath.Join(tmp, "foo.pid"),
				"sh", "-c", "echo $$; read",
			)

			in, err := pwd.StdinPipe()
			Ω(err).ShouldNot(HaveOccurred())

			session, err := Start(pwd, GinkgoWriter, GinkgoWriter)
			Ω(err).ShouldNot(HaveOccurred())

			Eventually(func() error {
				_, err := os.Stat(filepath.Join(tmp, "foo.pid"))
				return err
			}).Should(BeNil())

			read, err := ioutil.ReadFile(filepath.Join(tmp, "foo.pid"))
			Ω(err).ShouldNot(HaveOccurred())

			in.Write([]byte("\n"))

			Eventually(session).Should(Exit())
			Ω(string(read)).Should(Equal(string(session.Out.Contents())))

			_, err = os.Stat(filepath.Join(tmp, "foo.pid"))
			Ω(err).Should(HaveOccurred(), "pid file was not cleaned up")
		})
	})

	Context("when running a command as root", func() {
		It("executes with setuid and setgid", func() {
			sh := exec.Command(wsh, "--socket", socketPath, "--user", "root", "/bin/sh", "-c", "id -u; id -g")

			shSession, err := Start(sh, GinkgoWriter, GinkgoWriter)
			Ω(err).ShouldNot(HaveOccurred())

			Eventually(shSession).Should(Say("^0\n"))
			Eventually(shSession).Should(Say("^0\n"))
			Eventually(shSession).Should(Exit(0))
		})

		It("sets $HOME, $USER, and a $PATH with sbin dirs", func() {
			sh := exec.Command(wsh, "--socket", socketPath, "--user", "root", "/bin/sh", "-c", "env | sort")

			shSession, err := Start(sh, GinkgoWriter, GinkgoWriter)
			Ω(err).ShouldNot(HaveOccurred())

			Eventually(shSession).Should(Say("HOME=/root\n"))
			Eventually(shSession).Should(Say("PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin\n"))
			Eventually(shSession).Should(Say("USER=root\n"))
			Eventually(shSession).Should(Exit(0))
		})

		It("searches a sanitized path for the executable containing sbin directories", func() {
			onlyInSbin := exec.Command(wsh,
				"--socket", socketPath,
				"--user", "root",
				"ifconfig",
			)

			session, err := Start(onlyInSbin, GinkgoWriter, GinkgoWriter)
			Ω(err).ShouldNot(HaveOccurred())

			Eventually(session).Should(Exit(0))
		})

		It("executes in their home directory", func() {
			pwd := exec.Command(wsh, "--socket", socketPath, "--user", "root", "/bin/pwd")

			pwdSession, err := Start(pwd, GinkgoWriter, GinkgoWriter)
			Ω(err).ShouldNot(HaveOccurred())

			Eventually(pwdSession).Should(Say("/root\n"))
			Eventually(pwdSession).Should(Exit(0))
		})
	})

	Context("when piping stdin", func() {
		It("terminates when the input stream terminates", func() {
			sh := exec.Command(wsh, "--socket", socketPath, "/bin/sh")

			stdin, err := sh.StdinPipe()
			Ω(err).ShouldNot(HaveOccurred())

			shSession, err := Start(sh, GinkgoWriter, GinkgoWriter)
			Ω(err).ShouldNot(HaveOccurred())

			stdin.Write([]byte("echo hello"))
			stdin.Close()

			Eventually(shSession).Should(Say("hello\n"))
			Eventually(shSession).Should(Exit(0))
		})
	})

	Context("setting rlimits", func() {
		shouldSetRlimit := func(env []string, limitQueryCmd, expectedValue string) {
			ulimit := exec.Command(wsh, "--socket", socketPath, "--user", "root", "/bin/sh", "-c", limitQueryCmd)
			ulimit.Env = env

			ulimitSession, err := Start(ulimit, GinkgoWriter, GinkgoWriter)
			Ω(err).ShouldNot(HaveOccurred())

			Eventually(ulimitSession).Should(Say(expectedValue))
			Eventually(ulimitSession).Should(Exit(0))
		}

		var (
			rlimitResource               int      // the resource being limited, e.g. RLIMIT_CORE
			limit                        string   // the string suffix of the resource being limited, e.g. "CORE"
			limitValue                   uint64   // a (non-default) limit for the resource, e.g. 4096
			overrideEnv                  []string // environment specifying a (non-default) limit to wshd, e.g. ["RLIMIT_CORE=4096"]
			limitQueryCmd                string   // a command used to query a limit inside a container, e.g. "ulimit -c"
			expectedDefaultQueryResponse string   // the expected query response for the default limit, e.g. "0"
			expectedQueryResponse        string   // the expected query response for the non-default limit, e.g. "8"

			originalRlimit *syscall.Rlimit
		)

		JustBeforeEach(func() {
			overrideEnv = []string{fmt.Sprintf("RLIMIT_%s=%d", limit, limitValue)}
		})

		BeforeEach(func() {
			// Ensure the resource limit being tested is set to a low value.
			// beforeWshd is called just before wshd is launched.
			beforeWshd = func() {
				originalRlimit = getAndReduceRlimit(rlimitResource, limitValue/2)
			}
		})

		AfterEach(func() {
			Ω(syscall.Setrlimit(rlimitResource, originalRlimit)).Should(Succeed())
		})

		Describe("AS", func() {
			BeforeEach(func() {
				rlimitResource = RLIMIT_AS
				limit = "AS"
				limitValue = 2147483648 * 2

				limitQueryCmd = "ulimit -v"
				expectedDefaultQueryResponse = "unlimited"
				expectedQueryResponse = "4194304"
			})

			Context("when user namespacing is disabled", func() {
				BeforeEach(func() {
					userNs = "disabled"
				})
				It("defaults the rlimit when the environment variable is not set", func() {
					shouldSetRlimit([]string{}, limitQueryCmd, expectedDefaultQueryResponse)
				})
				It("overrides the rlimit when the environment variable is set", func() {
					shouldSetRlimit(overrideEnv, limitQueryCmd, expectedQueryResponse)
				})
			})

			Context("when user namespacing is enabled", func() {
				BeforeEach(func() {
					userNs = "enabled"
				})
				It("defaults the rlimit when the environment variable is not set", func() {
					shouldSetRlimit([]string{}, limitQueryCmd, expectedDefaultQueryResponse)
				})
				It("overrides the rlimit when the environment variable is set", func() {
					shouldSetRlimit(overrideEnv, limitQueryCmd, expectedQueryResponse)
				})
			})
		})

		Describe("CORE", func() {
			BeforeEach(func() {
				rlimitResource = RLIMIT_CORE
				limit = "CORE"
				limitValue = 4096

				limitQueryCmd = "ulimit -c"
				expectedDefaultQueryResponse = "0"
				expectedQueryResponse = "8"
			})

			Context("when user namespacing is disabled", func() {
				BeforeEach(func() {
					userNs = "disabled"
				})
				It("defaults the rlimit when the environment variable is not set", func() {
					shouldSetRlimit([]string{}, limitQueryCmd, expectedDefaultQueryResponse)
				})
				It("overrides the rlimit when the environment variable is set", func() {
					shouldSetRlimit(overrideEnv, limitQueryCmd, expectedQueryResponse)
				})
			})

			Context("when user namespacing is enabled", func() {
				BeforeEach(func() {
					userNs = "enabled"
				})
				It("defaults the rlimit when the environment variable is not set", func() {
					shouldSetRlimit([]string{}, limitQueryCmd, expectedDefaultQueryResponse)
				})
				It("overrides the rlimit when the environment variable is set", func() {
					shouldSetRlimit(overrideEnv, limitQueryCmd, expectedQueryResponse)
				})
			})
		})

		Describe("CPU", func() {
			BeforeEach(func() {
				rlimitResource = RLIMIT_CPU
				limit = "CPU"
				limitValue = 3600

				limitQueryCmd = "ulimit -t"
				expectedDefaultQueryResponse = "unlimited"
				expectedQueryResponse = "3600"
			})

			Context("when user namespacing is disabled", func() {
				BeforeEach(func() {
					userNs = "disabled"
				})
				It("defaults the rlimit when the environment variable is not set", func() {
					shouldSetRlimit([]string{}, limitQueryCmd, expectedDefaultQueryResponse)
				})
				It("overrides the rlimit when the environment variable is set", func() {
					shouldSetRlimit(overrideEnv, limitQueryCmd, expectedQueryResponse)
				})
			})

			Context("when user namespacing is enabled", func() {
				BeforeEach(func() {
					userNs = "enabled"
				})
				It("defaults the rlimit when the environment variable is not set", func() {
					shouldSetRlimit([]string{}, limitQueryCmd, expectedDefaultQueryResponse)
				})
				It("overrides the rlimit when the environment variable is set", func() {
					shouldSetRlimit(overrideEnv, limitQueryCmd, expectedQueryResponse)
				})
			})
		})

		Describe("DATA", func() {
			BeforeEach(func() {
				rlimitResource = RLIMIT_DATA
				limit = "DATA"
				limitValue = 1024 * 1024

				limitQueryCmd = "ulimit -d"
				expectedDefaultQueryResponse = "unlimited"
				expectedQueryResponse = "1024"
			})

			Context("when user namespacing is disabled", func() {
				BeforeEach(func() {
					userNs = "disabled"
				})
				It("defaults the rlimit when the environment variable is not set", func() {
					shouldSetRlimit([]string{}, limitQueryCmd, expectedDefaultQueryResponse)
				})
				It("overrides the rlimit when the environment variable is set", func() {
					shouldSetRlimit(overrideEnv, limitQueryCmd, expectedQueryResponse)
				})
			})

			Context("when user namespacing is enabled", func() {
				BeforeEach(func() {
					userNs = "enabled"
				})
				It("defaults the rlimit when the environment variable is not set", func() {
					shouldSetRlimit([]string{}, limitQueryCmd, expectedDefaultQueryResponse)
				})
				It("overrides the rlimit when the environment variable is set", func() {
					shouldSetRlimit(overrideEnv, limitQueryCmd, expectedQueryResponse)
				})
			})
		})

		Describe("FSIZE", func() {
			BeforeEach(func() {
				rlimitResource = RLIMIT_FSIZE
				limit = "FSIZE"
				limitValue = 4096 * 1024

				limitQueryCmd = "ulimit -f"
				expectedDefaultQueryResponse = "unlimited"
				expectedQueryResponse = "8192"
			})

			Context("when user namespacing is disabled", func() {
				BeforeEach(func() {
					userNs = "disabled"
				})
				It("defaults the rlimit when the environment variable is not set", func() {
					shouldSetRlimit([]string{}, limitQueryCmd, expectedDefaultQueryResponse)
				})
				It("overrides the rlimit when the environment variable is set", func() {
					shouldSetRlimit(overrideEnv, limitQueryCmd, expectedQueryResponse)
				})
			})

			Context("when user namespacing is enabled", func() {
				BeforeEach(func() {
					userNs = "enabled"
				})
				It("defaults the rlimit when the environment variable is not set", func() {
					shouldSetRlimit([]string{}, limitQueryCmd, expectedDefaultQueryResponse)
				})
				It("overrides the rlimit when the environment variable is set", func() {
					shouldSetRlimit(overrideEnv, limitQueryCmd, expectedQueryResponse)
				})
			})
		})

		Describe("LOCKS", func() {
			BeforeEach(func() {
				rlimitResource = RLIMIT_LOCKS
				limit = "LOCKS"
				limitValue = 1024

				limitQueryCmd = "ulimit -w"
				expectedDefaultQueryResponse = "unlimited"
				expectedQueryResponse = "1024"
			})

			Context("when user namespacing is disabled", func() {
				BeforeEach(func() {
					userNs = "disabled"
				})
				It("defaults the rlimit when the environment variable is not set", func() {
					shouldSetRlimit([]string{}, limitQueryCmd, expectedDefaultQueryResponse)
				})
				It("overrides the rlimit when the environment variable is set", func() {
					shouldSetRlimit(overrideEnv, limitQueryCmd, expectedQueryResponse)
				})
			})

			Context("when user namespacing is enabled", func() {
				BeforeEach(func() {
					userNs = "enabled"
				})
				It("defaults the rlimit when the environment variable is not set", func() {
					shouldSetRlimit([]string{}, limitQueryCmd, expectedDefaultQueryResponse)
				})
				It("overrides the rlimit when the environment variable is set", func() {
					shouldSetRlimit(overrideEnv, limitQueryCmd, expectedQueryResponse)
				})
			})
		})

		Describe("MEMLOCK", func() {
			BeforeEach(func() {
				rlimitResource = RLIMIT_MEMLOCK
				limit = "MEMLOCK"
				limitValue = 1024 * 32

				limitQueryCmd = "ulimit -l"
				expectedDefaultQueryResponse = "64"
				expectedQueryResponse = "32"
			})

			Context("when user namespacing is disabled", func() {
				BeforeEach(func() {
					userNs = "disabled"
				})
				It("defaults the rlimit when the environment variable is not set", func() {
					shouldSetRlimit([]string{}, limitQueryCmd, expectedDefaultQueryResponse)
				})
				It("overrides the rlimit when the environment variable is set", func() {
					shouldSetRlimit(overrideEnv, limitQueryCmd, expectedQueryResponse)
				})
			})

			Context("when user namespacing is enabled", func() {
				BeforeEach(func() {
					userNs = "enabled"
				})
				It("defaults the rlimit when the environment variable is not set", func() {
					shouldSetRlimit([]string{}, limitQueryCmd, expectedDefaultQueryResponse)
				})
				It("overrides the rlimit when the environment variable is set", func() {
					shouldSetRlimit(overrideEnv, limitQueryCmd, expectedQueryResponse)
				})
			})
		})

		Describe("MSGQUEUE", func() {
			BeforeEach(func() {
				rlimitResource = RLIMIT_MSGQUEUE
				limit = "MSGQUEUE"
				limitValue = 1024 * 100

				limitQueryCmd = "echo RLIMIT_MSGQUEUE not queryable"
				expectedDefaultQueryResponse = "RLIMIT_MSGQUEUE not queryable"
				expectedQueryResponse = "RLIMIT_MSGQUEUE not queryable"
			})

			Context("when user namespacing is disabled", func() {
				BeforeEach(func() {
					userNs = "disabled"
				})
				It("defaults the rlimit when the environment variable is not set", func() {
					shouldSetRlimit([]string{}, limitQueryCmd, expectedDefaultQueryResponse)
				})
				It("overrides the rlimit when the environment variable is set", func() {
					shouldSetRlimit(overrideEnv, limitQueryCmd, expectedQueryResponse)
				})
			})

			Context("when user namespacing is enabled", func() {
				BeforeEach(func() {
					userNs = "enabled"
				})
				It("defaults the rlimit when the environment variable is not set", func() {
					shouldSetRlimit([]string{}, limitQueryCmd, expectedDefaultQueryResponse)
				})
				It("overrides the rlimit when the environment variable is set", func() {
					shouldSetRlimit(overrideEnv, limitQueryCmd, expectedQueryResponse)
				})
			})
		})

		Describe("NICE", func() {
			BeforeEach(func() {
				rlimitResource = RLIMIT_NICE
				limit = "NICE"
				limitValue = 100

				limitQueryCmd = "ulimit -e"
				expectedDefaultQueryResponse = "0"
				expectedQueryResponse = "100"
			})

			Context("when user namespacing is disabled", func() {
				BeforeEach(func() {
					userNs = "disabled"
				})
				It("defaults the rlimit when the environment variable is not set", func() {
					shouldSetRlimit([]string{}, limitQueryCmd, expectedDefaultQueryResponse)
				})
				It("overrides the rlimit when the environment variable is set", func() {
					shouldSetRlimit(overrideEnv, limitQueryCmd, expectedQueryResponse)
				})
			})

			Context("when user namespacing is enabled", func() {
				BeforeEach(func() {
					userNs = "enabled"
				})
				It("defaults the rlimit when the environment variable is not set", func() {
					shouldSetRlimit([]string{}, limitQueryCmd, expectedDefaultQueryResponse)
				})
				It("overrides the rlimit when the environment variable is set", func() {
					shouldSetRlimit(overrideEnv, limitQueryCmd, expectedQueryResponse)
				})
			})
		})

		Describe("NOFILE", func() {
			BeforeEach(func() {
				rlimitResource = RLIMIT_NOFILE
				limit = "NOFILE"
				limitValue = 4096

				limitQueryCmd = "ulimit -n"
				expectedDefaultQueryResponse = "1024"
				expectedQueryResponse = "4096"
			})

			Context("when user namespacing is disabled", func() {
				BeforeEach(func() {
					userNs = "disabled"
				})
				It("defaults the rlimit when the environment variable is not set", func() {
					shouldSetRlimit([]string{}, limitQueryCmd, expectedDefaultQueryResponse)
				})
				It("overrides the rlimit when the environment variable is set", func() {
					shouldSetRlimit(overrideEnv, limitQueryCmd, expectedQueryResponse)
				})
			})

			Context("when user namespacing is enabled", func() {
				BeforeEach(func() {
					userNs = "enabled"
				})
				It("defaults the rlimit when the environment variable is not set", func() {
					shouldSetRlimit([]string{}, limitQueryCmd, expectedDefaultQueryResponse)
				})
				It("overrides the rlimit when the environment variable is set", func() {
					shouldSetRlimit(overrideEnv, limitQueryCmd, expectedQueryResponse)
				})
			})
		})

		Describe("NPROC", func() {
			BeforeEach(func() {
				rlimitResource = RLIMIT_NPROC
				limit = "NPROC"
				limitValue = 4096

				limitQueryCmd = "ulimit -p"
				expectedDefaultQueryResponse = "1024"
				expectedQueryResponse = "4096"
			})

			Context("when user namespacing is disabled", func() {
				BeforeEach(func() {
					userNs = "disabled"
				})
				It("defaults the rlimit when the environment variable is not set", func() {
					shouldSetRlimit([]string{}, limitQueryCmd, expectedDefaultQueryResponse)
				})
				It("overrides the rlimit when the environment variable is set", func() {
					shouldSetRlimit(overrideEnv, limitQueryCmd, expectedQueryResponse)
				})
			})

			Context("when user namespacing is enabled", func() {
				BeforeEach(func() {
					userNs = "enabled"
				})
				It("defaults the rlimit when the environment variable is not set", func() {
					shouldSetRlimit([]string{}, limitQueryCmd, expectedDefaultQueryResponse)
				})
				It("overrides the rlimit when the environment variable is set", func() {
					shouldSetRlimit(overrideEnv, limitQueryCmd, expectedQueryResponse)
				})
			})
		})

		Describe("RSS", func() {
			BeforeEach(func() {
				rlimitResource = RLIMIT_RSS
				limit = "RSS"
				limitValue = 4096 * 1024

				limitQueryCmd = "ulimit -m"
				expectedDefaultQueryResponse = "unlimited"
				expectedQueryResponse = "4096"
			})

			Context("when user namespacing is disabled", func() {
				BeforeEach(func() {
					userNs = "disabled"
				})
				It("defaults the rlimit when the environment variable is not set", func() {
					shouldSetRlimit([]string{}, limitQueryCmd, expectedDefaultQueryResponse)
				})
				It("overrides the rlimit when the environment variable is set", func() {
					shouldSetRlimit(overrideEnv, limitQueryCmd, expectedQueryResponse)
				})
			})

			Context("when user namespacing is enabled", func() {
				BeforeEach(func() {
					userNs = "enabled"
				})
				It("defaults the rlimit when the environment variable is not set", func() {
					shouldSetRlimit([]string{}, limitQueryCmd, expectedDefaultQueryResponse)
				})
				It("overrides the rlimit when the environment variable is set", func() {
					shouldSetRlimit(overrideEnv, limitQueryCmd, expectedQueryResponse)
				})
			})
		})

		Describe("RTPRIO", func() {
			BeforeEach(func() {
				rlimitResource = RLIMIT_RTPRIO
				limit = "RTPRIO"
				limitValue = 100

				limitQueryCmd = "ulimit -r"
				expectedDefaultQueryResponse = "0"
				expectedQueryResponse = "100"
			})

			Context("when user namespacing is disabled", func() {
				BeforeEach(func() {
					userNs = "disabled"
				})
				It("defaults the rlimit when the environment variable is not set", func() {
					shouldSetRlimit([]string{}, limitQueryCmd, expectedDefaultQueryResponse)
				})
				It("overrides the rlimit when the environment variable is set", func() {
					shouldSetRlimit(overrideEnv, limitQueryCmd, expectedQueryResponse)
				})
			})

			Context("when user namespacing is enabled", func() {
				BeforeEach(func() {
					userNs = "enabled"
				})
				It("defaults the rlimit when the environment variable is not set", func() {
					shouldSetRlimit([]string{}, limitQueryCmd, expectedDefaultQueryResponse)
				})
				It("overrides the rlimit when the environment variable is set", func() {
					shouldSetRlimit(overrideEnv, limitQueryCmd, expectedQueryResponse)
				})
			})
		})

		Describe("SIGPENDING", func() {
			BeforeEach(func() {
				rlimitResource = RLIMIT_SIGPENDING
				limit = "SIGPENDING"
				limitValue = 1024 * 4

				limitQueryCmd = "echo RLIMIT_SIGPENDING not queryable"
				expectedDefaultQueryResponse = "RLIMIT_SIGPENDING not queryable"
				expectedQueryResponse = "RLIMIT_SIGPENDING not queryable"
			})

			Context("when user namespacing is disabled", func() {
				BeforeEach(func() {
					userNs = "disabled"
				})
				It("defaults the rlimit when the environment variable is not set", func() {
					shouldSetRlimit([]string{}, limitQueryCmd, expectedDefaultQueryResponse)
				})
				It("overrides the rlimit when the environment variable is set", func() {
					shouldSetRlimit(overrideEnv, limitQueryCmd, expectedQueryResponse)
				})
			})

			Context("when user namespacing is enabled", func() {
				BeforeEach(func() {
					userNs = "enabled"
				})
				It("defaults the rlimit when the environment variable is not set", func() {
					shouldSetRlimit([]string{}, limitQueryCmd, expectedDefaultQueryResponse)
				})
				It("overrides the rlimit when the environment variable is set", func() {
					shouldSetRlimit(overrideEnv, limitQueryCmd, expectedQueryResponse)
				})
			})
		})

		Describe("STACK", func() {
			BeforeEach(func() {
				rlimitResource = RLIMIT_STACK
				limit = "STACK"
				limitValue = 4 * 1024 * 1024

				limitQueryCmd = "ulimit -s"
				expectedDefaultQueryResponse = "8192"
				expectedQueryResponse = "4096"
			})

			Context("when user namespacing is disabled", func() {
				BeforeEach(func() {
					userNs = "disabled"
				})
				It("defaults the rlimit when the environment variable is not set", func() {
					shouldSetRlimit([]string{}, limitQueryCmd, expectedDefaultQueryResponse)
				})
				It("overrides the rlimit when the environment variable is set", func() {
					shouldSetRlimit(overrideEnv, limitQueryCmd, expectedQueryResponse)
				})
			})

			Context("when user namespacing is enabled", func() {
				BeforeEach(func() {
					userNs = "enabled"
				})
				It("defaults the rlimit when the environment variable is not set", func() {
					shouldSetRlimit([]string{}, limitQueryCmd, expectedDefaultQueryResponse)
				})
				It("overrides the rlimit when the environment variable is set", func() {
					shouldSetRlimit(overrideEnv, limitQueryCmd, expectedQueryResponse)
				})
			})
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

func getAndReduceRlimit(rlimitResource int, limitVal uint64) *syscall.Rlimit {
	var curLimit syscall.Rlimit
	Ω(syscall.Getrlimit(rlimitResource, &curLimit)).Should(Succeed())

	Ω(syscall.Setrlimit(rlimitResource, &syscall.Rlimit{Cur: limitVal, Max: limitVal})).Should(Succeed())
	return &curLimit
}
