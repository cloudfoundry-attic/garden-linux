package networking_test

import (
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"strconv"
	"strings"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry/gunk/localip"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("IP settings", func() {
	var (
		container1 garden.Container
		container2 garden.Container

		containerNetwork1 string
		containerNetwork2 string

		containerInterface string

		gardenParms []string
	)

	BeforeEach(func() {
		container1 = nil
		container2 = nil
		containerNetwork1 = ""
		containerNetwork2 = ""

		gardenParms = []string{}
	})

	JustBeforeEach(func() {
		client = startGarden(gardenParms...)

		var err error
		container1, err = client.Create(garden.ContainerSpec{Network: containerNetwork1})
		Ω(err).ShouldNot(HaveOccurred())

		if len(containerNetwork2) > 0 {
			container2, err = client.Create(garden.ContainerSpec{Network: containerNetwork2})
			Ω(err).ShouldNot(HaveOccurred())
		}

		containerInterface = "w" + strconv.Itoa(GinkgoParallelNode()) + container1.Handle() + "-1"
	})

	AfterEach(func() {
		if container1 != nil {
			err := client.Destroy(container1.Handle())
			Ω(err).ShouldNot(HaveOccurred())
		}

		if container2 != nil {
			err := client.Destroy(container2.Handle())
			Ω(err).ShouldNot(HaveOccurred())
		}
	})

	Context("when the Network parameter is a subnet address", func() {
		BeforeEach(func() {
			containerNetwork1 = fmt.Sprintf("10.%d.0.0/24", GinkgoParallelNode())
		})

		Describe("container's network interface", func() {
			It("has the correct IP address", func() {
				stdout := gbytes.NewBuffer()
				stderr := gbytes.NewBuffer()

				process, err := container1.Run(garden.ProcessSpec{
					Path: "/sbin/ifconfig",
					Args: []string{containerInterface},
				}, garden.ProcessIO{
					Stdout: stdout,
					Stderr: stderr,
				})
				Ω(err).ShouldNot(HaveOccurred())
				rc, err := process.Wait()
				Ω(err).ShouldNot(HaveOccurred())
				Ω(rc).Should(Equal(0))

				Ω(stdout.Contents()).Should(ContainSubstring(fmt.Sprintf(" inet addr:10.%d.0.1 ", GinkgoParallelNode())))
			})
		})
	})

	Context("when the Network parameter is not a subnet address", func() {
		BeforeEach(func() {
			containerNetwork1 = fmt.Sprintf("10.%d.0.2/24", GinkgoParallelNode())
		})

		Describe("container's network interface", func() {
			It("has the specified IP address", func() {
				stdout := gbytes.NewBuffer()
				stderr := gbytes.NewBuffer()

				process, err := container1.Run(garden.ProcessSpec{
					Path: "/sbin/ifconfig",
					Args: []string{containerIfName(container1)},
				}, garden.ProcessIO{
					Stdout: stdout,
					Stderr: stderr,
				})
				Ω(err).ShouldNot(HaveOccurred())
				rc, err := process.Wait()
				Ω(err).ShouldNot(HaveOccurred())
				Ω(rc).Should(Equal(0))

				Ω(stdout.Contents()).Should(ContainSubstring(fmt.Sprintf(" inet addr:10.%d.0.2 ", GinkgoParallelNode())))
			})
		})
	})

	Describe("the container's network", func() {
		BeforeEach(func() {
			containerNetwork1 = fmt.Sprintf("10.%d.0.0/24", GinkgoParallelNode())
		})

		It("is reachable from the host", func() {
			info1, ierr := container1.Info()
			Ω(ierr).ShouldNot(HaveOccurred())

			out, err := exec.Command("/bin/ping", "-c 2", info1.ContainerIP).Output()
			Ω(out).Should(ContainSubstring(" 0% packet loss"))
			Ω(err).ShouldNot(HaveOccurred())
		})

		It("can reach external networks", func() {
			sender, err := container1.Run(garden.ProcessSpec{
				Path: "sh",
				Args: []string{"-c", fmt.Sprintf("nc -w4 %s 80", externalIP)},
			}, garden.ProcessIO{Stdout: GinkgoWriter, Stderr: GinkgoWriter})
			Ω(err).ShouldNot(HaveOccurred())

			Ω(sender.Wait()).Should(Equal(0))
		})
	})

	Describe("another container on the same subnet", func() {
		BeforeEach(func() {
			containerNetwork1 = fmt.Sprintf("10.%d.0.0/24", GinkgoParallelNode())
			containerNetwork2 = containerNetwork1
		})

		It("can reach the first container", func() {
			info1, err := container1.Info()
			Ω(err).ShouldNot(HaveOccurred())

			listener, err := container1.Run(garden.ProcessSpec{
				Path: "sh",
				Args: []string{"-c", "echo hi | nc -l -p 8080"},
			}, garden.ProcessIO{})
			Ω(err).ShouldNot(HaveOccurred())

			sender, err := container2.Run(garden.ProcessSpec{
				Path: "sh",
				Args: []string{"-c", fmt.Sprintf("echo hello | nc -w1 %s 8080", info1.ContainerIP)},
			}, garden.ProcessIO{})
			Ω(err).ShouldNot(HaveOccurred())

			Ω(sender.Wait()).Should(Equal(0))
			Ω(listener.Wait()).Should(Equal(0))
		})

		It("can be reached from the host", func() {
			info2, ierr := container2.Info()
			Ω(ierr).ShouldNot(HaveOccurred())

			out, err := exec.Command("/bin/ping", "-c 2", info2.ContainerIP).Output()
			Ω(out).Should(ContainSubstring(" 0% packet loss"))
			Ω(err).ShouldNot(HaveOccurred())
		})

		It("can reach external networks", func() {
			sender, err := container2.Run(garden.ProcessSpec{
				Path: "sh",
				Args: []string{"-c", fmt.Sprintf("nc -w4 %s 80", externalIP)},
			}, garden.ProcessIO{Stdout: GinkgoWriter, Stderr: GinkgoWriter})
			Ω(err).ShouldNot(HaveOccurred())

			Ω(sender.Wait()).Should(Equal(0))
		})

		Context("when the first container is deleted", func() {
			JustBeforeEach(func() {
				Ω(client.Destroy(container1.Handle())).Should(Succeed())
				container1 = nil
			})

			Context("the second container", func() {
				It("can still reach external networks", func() {
					sender, err := container2.Run(garden.ProcessSpec{
						Path: "sh",
						Args: []string{"-c", fmt.Sprintf("nc -w4 %s 80", externalIP)},
					}, garden.ProcessIO{Stdout: GinkgoWriter, Stderr: GinkgoWriter})
					Ω(err).ShouldNot(HaveOccurred())

					Ω(sender.Wait()).Should(Equal(0))
				})

				It("can still be reached from the host", func() {
					info2, ierr := container2.Info()
					Ω(ierr).ShouldNot(HaveOccurred())

					out, err := exec.Command("/bin/ping", "-c 2", info2.ContainerIP).Output()
					Ω(out).Should(ContainSubstring(" 0% packet loss"))
					Ω(err).ShouldNot(HaveOccurred())
				})
			})

			Context("a newly created container in the same subnet", func() {
				var (
					container3 garden.Container
				)

				JustBeforeEach(func() {
					var err error
					container3, err = client.Create(garden.ContainerSpec{Network: containerNetwork1})
					Ω(err).ShouldNot(HaveOccurred())
				})

				AfterEach(func() {
					err := client.Destroy(container3.Handle())
					Ω(err).ShouldNot(HaveOccurred())
				})

				It("can reach external networks", func() {
					sender, err := container3.Run(garden.ProcessSpec{
						Path: "sh",
						Args: []string{"-c", fmt.Sprintf("nc -w4 %s 80", externalIP)},
					}, garden.ProcessIO{Stdout: GinkgoWriter, Stderr: GinkgoWriter})
					Ω(err).ShouldNot(HaveOccurred())

					Ω(sender.Wait()).Should(Equal(0))
				})

				It("can reach the second container", func() {
					info2, err := container2.Info()
					Ω(err).ShouldNot(HaveOccurred())

					listener, err := container2.Run(garden.ProcessSpec{
						Path: "sh",
						Args: []string{"-c", "echo hi | nc -l -p 8080"},
					}, garden.ProcessIO{})
					Ω(err).ShouldNot(HaveOccurred())

					sender, err := container3.Run(garden.ProcessSpec{
						Path: "sh",
						Args: []string{"-c", fmt.Sprintf("echo hello | nc -w1 %s 8080", info2.ContainerIP)},
					}, garden.ProcessIO{})
					Ω(err).ShouldNot(HaveOccurred())

					Ω(sender.Wait()).Should(Equal(0))
					Ω(listener.Wait()).Should(Equal(0))
				})

				It("can be reached from the host", func() {
					info3, ierr := container3.Info()
					Ω(ierr).ShouldNot(HaveOccurred())

					out, err := exec.Command("/bin/ping", "-c 2", info3.ContainerIP).Output()
					Ω(out).Should(ContainSubstring(" 0% packet loss"))
					Ω(err).ShouldNot(HaveOccurred())
				})
			})
		})
	})

	Describe("host's network", func() {
		Context("when host access is explicitly allowed", func() {
			BeforeEach(func() {
				containerNetwork1 = fmt.Sprintf("10.%d.0.8/30", GinkgoParallelNode())
				gardenParms = []string{"-allowHostAccess=true"}
			})

			It("is reachable from inside the container", func() {
				checkHostAccess(container1, true)
			})
		})

		Context("when host access is explicitly disallowed", func() {
			BeforeEach(func() {
				containerNetwork1 = fmt.Sprintf("10.%d.0.8/30", GinkgoParallelNode())
				gardenParms = []string{"-allowHostAccess=false"}
			})

			It("is not reachable from inside the container", func() {
				checkHostAccess(container1, false)
			})
		})

		Context("when host access is implicitly disallowed", func() {
			BeforeEach(func() {
				containerNetwork1 = fmt.Sprintf("10.%d.0.8/30", GinkgoParallelNode())
			})

			It("is not reachable from inside the container", func() {
				checkHostAccess(container1, false)
			})
		})
	})

	Describe("the container's external ip", func() {
		It("is the external IP of its host", func() {
			info1, err := container1.Info()
			Ω(err).ShouldNot(HaveOccurred())

			localIP, err := localip.LocalIP()
			Ω(err).ShouldNot(HaveOccurred())

			Ω(localIP).Should(Equal(info1.ExternalIP))
		})
	})
})

func checkHostAccess(container garden.Container, permitted bool) {
	info1, ierr := container.Info()
	Ω(ierr).ShouldNot(HaveOccurred())

	stdout := gbytes.NewBuffer()
	stderr := gbytes.NewBuffer()

	listener, err := net.Listen("tcp", fmt.Sprintf("%s:0", info1.HostIP))
	Ω(err).ShouldNot(HaveOccurred())
	defer listener.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Hello")
	})

	go (&http.Server{Handler: mux}).Serve(listener)

	process, err := container.Run(garden.ProcessSpec{
		Path: "sh",
		Args: []string{"-c", fmt.Sprintf("(echo 'GET /test HTTP/1.1'; echo 'Host: foo.com'; echo) | nc %s %s", info1.HostIP, strings.Split(listener.Addr().String(), ":")[1])},
	}, garden.ProcessIO{
		Stdout: stdout,
		Stderr: stderr,
	})
	Ω(err).ShouldNot(HaveOccurred())

	rc, err := process.Wait()
	Ω(err).ShouldNot(HaveOccurred())

	if permitted {
		Ω(rc).Should(Equal(0))
		Ω(stdout.Contents()).Should(ContainSubstring("Hello"))
	} else {
		Ω(rc).Should(Equal(1))
	}
}
