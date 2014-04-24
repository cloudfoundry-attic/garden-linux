package gordon_test

import (
	"bytes"
	"errors"
	"runtime"

	"github.com/cloudfoundry-incubator/gordon/fake_gordon"

	. "github.com/cloudfoundry-incubator/gordon"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"code.google.com/p/gogoprotobuf/proto"
	warden "github.com/cloudfoundry-incubator/garden/protocol"
)

var _ = Describe("Client", func() {
	var (
		client      Client
		writeBuffer *bytes.Buffer
		provider    *FakeConnectionProvider
	)

	BeforeEach(func() {
		writeBuffer = new(bytes.Buffer)

	})

	stdout := warden.ProcessPayload_stdout
	stderr := warden.ProcessPayload_stderr

	It("should have a fake", func() {
		client = fake_gordon.New()
	})

	Describe("Connect", func() {
		Context("with a successful provider", func() {
			BeforeEach(func() {
				client = NewClient(NewFakeConnectionProvider(new(bytes.Buffer), new(bytes.Buffer)))
			})

			It("should connect", func() {
				err := client.Connect()
				Ω(err).ShouldNot(HaveOccurred())
			})
		})

		Context("with a failing provider", func() {
			BeforeEach(func() {
				client = NewClient(&FailingConnectionProvider{})
			})

			It("should fail to connect", func() {
				err := client.Connect()
				Ω(err).Should(Equal(errors.New("nope!")))
			})
		})
	})

	Describe("The container lifecycle", func() {
		BeforeEach(func() {
			provider = NewFakeConnectionProvider(
				warden.Messages(
					&warden.CreateResponse{Handle: proto.String("foo")},
					&warden.StopResponse{},
					&warden.DestroyResponse{},
				),
				writeBuffer,
			)

			client = NewClient(provider)
			err := client.Connect()
			Ω(err).ShouldNot(HaveOccurred())
		})

		It("should be able to create, stop and destroy a container", func() {
			res, err := client.Create(map[string]string{
				"foo": "bar",
			})
			Ω(err).ShouldNot(HaveOccurred())
			Ω(res.GetHandle()).Should(Equal("foo"))

			_, err = client.Stop("foo", true, true)
			Ω(err).ShouldNot(HaveOccurred())

			_, err = client.Destroy("foo")
			Ω(err).ShouldNot(HaveOccurred())

			expectedWriteBufferContents := string(warden.Messages(
				&warden.CreateRequest{
					Properties: []*warden.Property{
						{
							Key:   proto.String("foo"),
							Value: proto.String("bar"),
						},
					},
				},
				&warden.StopRequest{
					Handle:     proto.String("foo"),
					Background: proto.Bool(true),
					Kill:       proto.Bool(true),
				},
				&warden.DestroyRequest{Handle: proto.String("foo")},
			).Bytes())

			Ω(string(writeBuffer.Bytes())).Should(Equal(expectedWriteBufferContents))
		})
	})

	Describe("Running", func() {
		BeforeEach(func() {
			provider = NewFakeConnectionProvider(
				warden.Messages(
					&warden.ProcessPayload{
						ProcessId: proto.Uint32(1721),
					},
					&warden.ProcessPayload{
						ProcessId: proto.Uint32(1721),
						Source:    &stdout,
						Data:      proto.String("some data for stdout"),
					},
					&warden.ProcessPayload{
						ProcessId:  proto.Uint32(1721),
						ExitStatus: proto.Uint32(42),
					},
				),
				writeBuffer,
			)

			client = NewClient(provider)
			err := client.Connect()
			Ω(err).ShouldNot(HaveOccurred())
		})

		It("should spawn and stream succesfully", func(done Done) {
			processID, responses, err := client.Run(
				"foo",
				"echo some data for stdout",
				ResourceLimits{FileDescriptors: 72},
				[]EnvironmentVariable{
					EnvironmentVariable{Key: "BREAKFAST", Value: "Everything Bagel"},
					EnvironmentVariable{Key: "LUNCH", Value: "BLT"},
				},
			)
			Ω(err).ShouldNot(HaveOccurred())
			Ω(processID).Should(BeNumerically("==", 1721))

			expectedWriteBufferContents := string(warden.Messages(
				&warden.RunRequest{
					Handle:  proto.String("foo"),
					Script:  proto.String("echo some data for stdout"),
					Rlimits: &warden.ResourceLimits{Nofile: proto.Uint64(72)},
					Env: []*warden.EnvironmentVariable{
						&warden.EnvironmentVariable{
							Key:   proto.String("BREAKFAST"),
							Value: proto.String("Everything Bagel"),
						},
						&warden.EnvironmentVariable{
							Key:   proto.String("LUNCH"),
							Value: proto.String("BLT"),
						},
					},
				},
			).Bytes())

			Ω(string(writeBuffer.Bytes())).Should(Equal(expectedWriteBufferContents))

			res := <-responses
			Ω(res.GetSource()).Should(Equal(stdout))
			Ω(res.GetData()).Should(Equal("some data for stdout"))

			res = <-responses
			Ω(res.GetExitStatus()).Should(BeNumerically("==", 42))

			Eventually(responses).Should(BeClosed())

			close(done)
		})

		Context("When resource limits are set to 0", func() {
			It("should not populate the ResourceLimits in the protocol buffer", func() {
				processID, _, err := client.Run("foo", "echo some data for stdout", ResourceLimits{FileDescriptors: 0}, nil)
				Ω(err).ShouldNot(HaveOccurred())
				Ω(processID).Should(BeNumerically("==", 1721))

				expectedWriteBufferContents := string(warden.Messages(
					&warden.RunRequest{
						Handle:  proto.String("foo"),
						Script:  proto.String("echo some data for stdout"),
						Rlimits: &warden.ResourceLimits{},
					},
				).Bytes())

				Ω(string(writeBuffer.Bytes())).Should(Equal(expectedWriteBufferContents))
			})
		})
	})

	Describe("Attaching", func() {
		BeforeEach(func() {
			provider = NewFakeConnectionProvider(
				warden.Messages(
					&warden.ProcessPayload{
						ProcessId: proto.Uint32(1721),
						Source:    &stdout,
						Data:      proto.String("some data for stdout"),
					},
					&warden.ProcessPayload{
						ProcessId: proto.Uint32(1721),
						Source:    &stderr,
						Data:      proto.String("some data for stderr"),
					},
					&warden.ProcessPayload{
						ProcessId:  proto.Uint32(1721),
						ExitStatus: proto.Uint32(42),
					},
				),
				writeBuffer,
			)

			client = NewClient(provider)
			err := client.Connect()
			Ω(err).ShouldNot(HaveOccurred())
		})

		It("should spawn and stream succesfully", func(done Done) {
			responses, err := client.Attach("foo", 1721)
			Ω(err).ShouldNot(HaveOccurred())

			expectedWriteBufferContents := string(warden.Messages(
				&warden.AttachRequest{
					Handle:    proto.String("foo"),
					ProcessId: proto.Uint32(1721),
				},
			).Bytes())

			Ω(string(writeBuffer.Bytes())).Should(Equal(expectedWriteBufferContents))

			res := <-responses
			Ω(res.GetSource()).Should(Equal(stdout))
			Ω(res.GetData()).Should(Equal("some data for stdout"))

			res = <-responses
			Ω(res.GetSource()).Should(Equal(stderr))
			Ω(res.GetData()).Should(Equal("some data for stderr"))

			res = <-responses
			Ω(res.GetExitStatus()).Should(BeNumerically("==", 42))

			Eventually(responses).Should(BeClosed())

			close(done)
		})
	})

	Describe("LimitingCPU", func() {
		BeforeEach(func() {
			provider = NewFakeConnectionProvider(
				warden.Messages(
					&warden.LimitCpuResponse{},
				),
				writeBuffer,
			)

			client = NewClient(provider)
			err := client.Connect()
			Ω(err).ShouldNot(HaveOccurred())
		})

		It("limits CPU shares", func() {
			_, err := client.LimitCPU("foo", 10)
			Ω(err).ShouldNot(HaveOccurred())

			expectedWriteBufferContents := string(warden.Messages(
				&warden.LimitCpuRequest{
					Handle:        proto.String("foo"),
					LimitInShares: proto.Uint64(10),
				},
			).Bytes())

			Ω(string(writeBuffer.Bytes())).Should(Equal(expectedWriteBufferContents))
		})
	})

	Describe("LimitingDisk", func() {
		BeforeEach(func() {
			provider = NewFakeConnectionProvider(
				warden.Messages(
					&warden.LimitDiskResponse{},
				),
				writeBuffer,
			)

			client = NewClient(provider)
			err := client.Connect()
			Ω(err).ShouldNot(HaveOccurred())
		})

		Context("when both byte limit and inode limit are specified", func() {
			It("should limit them both", func() {
				_, err := client.LimitDisk("foo", DiskLimits{
					ByteLimit:  10,
					InodeLimit: 3,
				})
				Ω(err).ShouldNot(HaveOccurred())

				expectedWriteBufferContents := string(warden.Messages(
					&warden.LimitDiskRequest{
						Handle:     proto.String("foo"),
						ByteLimit:  proto.Uint64(10),
						InodeLimit: proto.Uint64(3),
					},
				).Bytes())

				Ω(string(writeBuffer.Bytes())).Should(Equal(expectedWriteBufferContents))
			})
		})

		Context("when only the byte limit is specified", func() {
			It("should limit the bytes only", func() {
				_, err := client.LimitDisk("foo", DiskLimits{
					ByteLimit: 10,
				})
				Ω(err).ShouldNot(HaveOccurred())

				expectedWriteBufferContents := string(warden.Messages(
					&warden.LimitDiskRequest{
						Handle:    proto.String("foo"),
						ByteLimit: proto.Uint64(10),
					},
				).Bytes())

				Ω(string(writeBuffer.Bytes())).Should(Equal(expectedWriteBufferContents))
			})
		})

		Context("when only the inode limit is specified", func() {
			It("should limit the inodes only", func() {
				_, err := client.LimitDisk("foo", DiskLimits{
					InodeLimit: 2,
				})
				Ω(err).ShouldNot(HaveOccurred())

				expectedWriteBufferContents := string(warden.Messages(
					&warden.LimitDiskRequest{
						Handle:     proto.String("foo"),
						InodeLimit: proto.Uint64(2),
					},
				).Bytes())

				Ω(string(writeBuffer.Bytes())).Should(Equal(expectedWriteBufferContents))
			})
		})
	})

	Describe("Querying containers", func() {
		Describe("Listing containers", func() {
			BeforeEach(func() {
				provider = NewFakeConnectionProvider(
					warden.Messages(
						&warden.ListResponse{
							Handles: []string{"container1", "container6"},
						},
					),
					writeBuffer,
				)

				client = NewClient(provider)
				err := client.Connect()
				Ω(err).ShouldNot(HaveOccurred())
			})

			It("should list the containers", func() {
				res, err := client.List(map[string]string{"foo": "bar"})
				Ω(err).ShouldNot(HaveOccurred())
				Ω(res.GetHandles()).Should(Equal([]string{"container1", "container6"}))

				expectedWriteBufferContents := string(warden.Messages(
					&warden.ListRequest{
						Properties: []*warden.Property{
							{
								Key:   proto.String("foo"),
								Value: proto.String("bar"),
							},
						},
					},
				).Bytes())

				Ω(string(writeBuffer.Bytes())).Should(Equal(expectedWriteBufferContents))
			})
		})

		Describe("Getting info for a specific container", func() {
			BeforeEach(func() {
				provider = NewFakeConnectionProvider(
					warden.Messages(
						&warden.InfoResponse{
							State: proto.String("stopped"),
						},
					),
					writeBuffer,
				)

				client = NewClient(provider)
				err := client.Connect()
				Ω(err).ShouldNot(HaveOccurred())
			})

			It("should return info for the requested handle", func() {
				res, err := client.Info("handle")

				Ω(err).ShouldNot(HaveOccurred())
				Ω(res.GetState()).Should(Equal("stopped"))

				expectedWriteBufferContents := string(warden.Messages(
					&warden.InfoRequest{
						Handle: proto.String("handle"),
					},
				).Bytes())

				Ω(string(writeBuffer.Bytes())).Should(Equal(expectedWriteBufferContents))
			})
		})

		Describe("Reconnecting", func() {
			var (
				firstWriteBuffer  *bytes.Buffer
				secondWriteBuffer *bytes.Buffer
			)

			BeforeEach(func() {
				firstWriteBuffer = bytes.NewBuffer([]byte{})
				secondWriteBuffer = bytes.NewBuffer([]byte{})

				mcp := &ManyConnectionProvider{
					ConnectionProviders: []ConnectionProvider{
						NewFakeConnectionProvider(
							warden.Messages(
								&warden.CreateResponse{Handle: proto.String("handle a")},
								// disconnect
							),
							firstWriteBuffer,
						),
						NewFakeConnectionProvider(
							warden.Messages(
								&warden.CreateResponse{Handle: proto.String("handle b")},
								&warden.DestroyResponse{},
								&warden.DestroyResponse{},
							),
							secondWriteBuffer,
						),
					},
				}

				client = NewClient(mcp)
				err := client.Connect()
				Ω(err).ShouldNot(HaveOccurred())
			})

			It("should attempt to reconnect when a disconnect occurs", func() {
				c1, err := client.Create(nil)
				Ω(err).ShouldNot(HaveOccurred())

				// let client notice disconnect
				runtime.Gosched()

				c2, err := client.Create(nil)
				Ω(err).ShouldNot(HaveOccurred())

				_, err = client.Destroy(c1.GetHandle())
				Ω(err).ShouldNot(HaveOccurred())

				_, err = client.Destroy(c2.GetHandle())
				Ω(err).ShouldNot(HaveOccurred())

				expectedWriteBufferContents := string(warden.Messages(
					&warden.CreateRequest{},
				).Bytes())

				Ω(string(firstWriteBuffer.Bytes())).Should(Equal(expectedWriteBufferContents))

				expectedWriteBufferContents = string(warden.Messages(
					&warden.CreateRequest{},
					&warden.DestroyRequest{
						Handle: proto.String("handle a"),
					},
					&warden.DestroyRequest{
						Handle: proto.String("handle b"),
					},
				).Bytes())

				Ω(string(secondWriteBuffer.Bytes())).Should(Equal(expectedWriteBufferContents))
			})
		})
	})
})
