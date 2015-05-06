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
		Expect(err).ToNot(HaveOccurred())

		if len(containerNetwork2) > 0 {
			container2, err = client.Create(garden.ContainerSpec{Network: containerNetwork2})
			Expect(err).ToNot(HaveOccurred())
		}

		containerInterface = "w" + strconv.Itoa(GinkgoParallelNode()) + container1.Handle() + "-1"
	})

	AfterEach(func() {
		if container1 != nil {
			err := client.Destroy(container1.Handle())
			Expect(err).ToNot(HaveOccurred())
		}

		if container2 != nil {
			err := client.Destroy(container2.Handle())
			Expect(err).ToNot(HaveOccurred())
		}
	})

	PContext("when the Network parameter is a subnet address", func() {
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
				Expect(err).ToNot(HaveOccurred())
				rc, err := process.Wait()
				Expect(err).ToNot(HaveOccurred())
				Expect(rc).To(Equal(0))

				Expect(stdout.Contents()).To(ContainSubstring(fmt.Sprintf(" inet addr:10.%d.0.1 ", GinkgoParallelNode())))
			})
		})
	})

	PContext("when the Network parameter is not a subnet address", func() {
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
				Expect(err).ToNot(HaveOccurred())
				rc, err := process.Wait()
				Expect(err).ToNot(HaveOccurred())
				Expect(rc).To(Equal(0))

				Expect(stdout.Contents()).To(ContainSubstring(fmt.Sprintf(" inet addr:10.%d.0.2 ", GinkgoParallelNode())))
			})
		})
	})

	PDescribe("the container's network", func() {
		BeforeEach(func() {
			containerNetwork1 = fmt.Sprintf("10.%d.0.0/24", GinkgoParallelNode())
		})

		It("is reachable from the host", func() {
			info1, ierr := container1.Info()
			Expect(ierr).ToNot(HaveOccurred())

			out, err := exec.Command("/bin/ping", "-c 2", info1.ContainerIP).Output()
			Expect(out).To(ContainSubstring(" 0% packet loss"))
			Expect(err).ToNot(HaveOccurred())
		})
	})

	PDescribe("another container on the same subnet", func() {
		BeforeEach(func() {
			containerNetwork1 = fmt.Sprintf("10.%d.0.0/24", GinkgoParallelNode())
			containerNetwork2 = containerNetwork1
		})

		Context("when the first container is deleted", func() {
			JustBeforeEach(func() {
				Expect(client.Destroy(container1.Handle())).To(Succeed())
				container1 = nil
			})

			Context("the second container", func() {
				It("can still reach external networks", func() {
					Expect(checkInternet(container2)).To(Succeed())
				})

				It("can still be reached from the host", func() {
					info2, ierr := container2.Info()
					Expect(ierr).ToNot(HaveOccurred())

					out, err := exec.Command("/bin/ping", "-c 2", info2.ContainerIP).Output()
					Expect(out).To(ContainSubstring(" 0% packet loss"))
					Expect(err).ToNot(HaveOccurred())
				})
			})

			Context("a newly created container in the same subnet", func() {
				var (
					container3 garden.Container
				)

				JustBeforeEach(func() {
					var err error
					container3, err = client.Create(garden.ContainerSpec{Network: containerNetwork1})
					Expect(err).ToNot(HaveOccurred())
				})

				AfterEach(func() {
					err := client.Destroy(container3.Handle())
					Expect(err).ToNot(HaveOccurred())
				})

				It("can reach the second container", func() {
					info2, err := container2.Info()
					Expect(err).ToNot(HaveOccurred())

					listener, err := container2.Run(garden.ProcessSpec{
						Path: "sh",
						Args: []string{"-c", "echo hi | nc -l -p 8080"},
					}, garden.ProcessIO{})
					Expect(err).ToNot(HaveOccurred())

					Expect(checkConnection(container3, info2.ContainerIP, 8080)).To(Succeed())

					Expect(listener.Wait()).To(Equal(0))
				})

				It("can be reached from the host", func() {
					info3, ierr := container3.Info()
					Expect(ierr).ToNot(HaveOccurred())

					out, err := exec.Command("/bin/ping", "-c 2", info3.ContainerIP).Output()
					Expect(out).To(ContainSubstring(" 0% packet loss"))
					Expect(err).ToNot(HaveOccurred())
				})
			})
		})
	})

	PDescribe("host's network", func() {
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

	PDescribe("the container's external ip", func() {
		It("is the external IP of its host", func() {
			info1, err := container1.Info()
			Expect(err).ToNot(HaveOccurred())

			localIP, err := localip.LocalIP()
			Expect(err).ToNot(HaveOccurred())

			Expect(localIP).To(Equal(info1.ExternalIP))
		})
	})
})

func checkHostAccess(container garden.Container, permitted bool) {
	info1, ierr := container.Info()
	Expect(ierr).ToNot(HaveOccurred())

	listener, err := net.Listen("tcp", fmt.Sprintf("%s:0", info1.HostIP))
	Expect(err).ToNot(HaveOccurred())
	defer listener.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Hello")
	})

	go (&http.Server{Handler: mux}).Serve(listener)

	port, err := strconv.Atoi(strings.Split(listener.Addr().String(), ":")[1])
	Expect(err).ToNot(HaveOccurred())
	err = checkConnection(container, info1.HostIP, port)

	if permitted {
		Expect(err).ToNot(HaveOccurred())
	} else {
		Expect(err).To(HaveOccurred())
	}
}
