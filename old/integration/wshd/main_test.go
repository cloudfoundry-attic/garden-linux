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
	"syscall"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gbytes"
	. "github.com/onsi/gomega/gexec"
)

var _ = Describe("Running wshd", func() {
	wshd := "../../linux_backend/skeleton/bin/wshd"

	wsh := "../../linux_backend/skeleton/bin/wsh"

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
		Expect(err).ToNot(HaveOccurred())

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
		Expect(err).ToNot(HaveOccurred())

		hookPath, err := Build("github.com/cloudfoundry-incubator/garden-linux/old/integration/wshd/fake_hook")
		Expect(err).ToNot(HaveOccurred())
		err = copyFile(hookPath, path.Join(libDir, "hook"))
		Expect(err).ToNot(HaveOccurred())

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
		Expect(err).ToNot(HaveOccurred())
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
		Expect(err).ToNot(HaveOccurred())

		Eventually(wshdSession, 30).Should(Exit(0))

		Eventually(ErrorDialingUnix(socketPath)).ShouldNot(HaveOccurred())
	})

	AfterEach(func() {
		wshdPidfile, err := os.Open(path.Join(containerPath, "run", "wshd.pid"))
		Expect(err).ToNot(HaveOccurred())

		var wshdPid int
		_, err = fmt.Fscanf(wshdPidfile, "%d", &wshdPid)
		Expect(err).ToNot(HaveOccurred())

		proc, err := os.FindProcess(wshdPid)
		Expect(err).ToNot(HaveOccurred())

		err = proc.Kill()
		Expect(err).ToNot(HaveOccurred())

		for _, submount := range []string{"dev", "etc", "home", "sbin", "var", "tmp"} {
			mountPoint := path.Join(containerPath, "mnt", submount)

			err := syscall.Unmount(mountPoint, 0)
			Expect(err).ToNot(HaveOccurred())
		}

		err = syscall.Unmount(path.Join(containerPath, "mnt"), 0)
		Expect(err).ToNot(HaveOccurred())

		Eventually(func() error {
			return os.RemoveAll(containerPath)
		}, 10).ShouldNot(HaveOccurred())
	})

	PContext("setting rlimits", func() {
		shouldSetRlimit := func(env []string, limitQueryCmd, expectedValue string) {
			ulimit := exec.Command(wsh, "--socket", socketPath, "--user", "root", "/bin/sh", "-c", limitQueryCmd)
			ulimit.Env = env

			ulimitSession, err := Start(ulimit, GinkgoWriter, GinkgoWriter)
			Expect(err).ToNot(HaveOccurred())

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
			Expect(syscall.Setrlimit(rlimitResource, originalRlimit)).To(Succeed())
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
	Expect(syscall.Getrlimit(rlimitResource, &curLimit)).To(Succeed())

	Expect(syscall.Setrlimit(rlimitResource, &syscall.Rlimit{Cur: limitVal, Max: limitVal})).To(Succeed())
	return &curLimit
}
