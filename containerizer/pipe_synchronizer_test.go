package containerizer_test

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"os"
	"time"

	"github.com/cloudfoundry-incubator/garden-linux/containerizer"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("PipeSynchronizer", func() {
	var pipeSynchronizer *containerizer.PipeSynchronizer
	var reader, writer *os.File

	ErrorSignal := containerizer.Signal{Type: containerizer.SignalError,
		Message: "error: Bang Bang"}
	SuccessSignal := containerizer.Signal{Type: containerizer.SignalSuccess}

	BeforeEach(func() {
		var err error

		reader, writer, err = os.Pipe()
		Expect(err).ToNot(HaveOccurred())

		pipeSynchronizer = &containerizer.PipeSynchronizer{
			Reader: reader,
			Writer: writer,
		}
	})

	AfterEach(func() {
		reader.Close()
		writer.Close()
	})

	Describe("Wait", func() {
		Context("when the buffer is signaled", func() {
			Context("with successful signal", func() {
				It("succeeds", func() {
					message, err := json.Marshal(SuccessSignal)
					Expect(err).ToNot(HaveOccurred())
					writer.Write(message)

					err = pipeSynchronizer.Wait(time.Second * 1)
					Expect(err).ToNot(HaveOccurred())

					signal, err := readSignal(reader, time.Millisecond*500)
					Expect(err).To(MatchError("Reached timeout"))
					Expect(signal).To(Equal(containerizer.Signal{}))
				})
			})

			Context("with error signal", func() {
				It("returns the error", func() {
					message, err := json.Marshal(ErrorSignal)
					Expect(err).ToNot(HaveOccurred())
					writer.Write(message)

					err = pipeSynchronizer.Wait(time.Second * 1)
					Expect(err).To(MatchError("error: Bang Bang"))
					Expect(err).To(BeAssignableToTypeOf(&containerizer.PipeSynchronizerError{}))
				})
			})
		})

		Context("when the buffer is not signaled", func() {
			It("times out gracefully", func() {
				err := pipeSynchronizer.Wait(time.Second * 1)
				Expect(err).To(MatchError("synchronizer wait timeout"))
			})
		})

		Context("when received a wrong signal data", func() {
			It("returns an error", func() {
				writer.Write([]byte("Hasta Lavista"))

				err := pipeSynchronizer.Wait(time.Second * 1)
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("IsSignalError", func() {
		Context("when the error is SignalError", func() {
			It("returns true", func() {
				result := pipeSynchronizer.IsSignalError(&containerizer.PipeSynchronizerError{})
				Expect(result).To(Equal(true))
			})
		})
		Context("when the error is not SignalError", func() {
			It("returns false", func() {
				result := pipeSynchronizer.IsSignalError(errors.New("Bump"))
				Expect(result).To(Equal(false))
			})
		})
	})

	Describe("SignalSuccess", func() {

		It("sends signal successfully", func() {
			err := pipeSynchronizer.SignalSuccess()
			Expect(err).ToNot(HaveOccurred())

			signal, err := readSignal(reader, time.Second*3)
			Expect(err).ToNot(HaveOccurred())
			Expect(signal).To(Equal(SuccessSignal))
		})

		Context("when writer is not writtable", func() {
			It("returns an error", func() {
				file, err := ioutil.TempFile("", "")
				Expect(err).ToNot(HaveOccurred())

				writer, err := os.Open(file.Name())
				Expect(err).ToNot(HaveOccurred())

				pipeSynchronizer.Writer = writer

				err = pipeSynchronizer.SignalSuccess()
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("SignalError", func() {
		It("sends signal successfully", func() {
			err := pipeSynchronizer.SignalError(errors.New("Bang Bang"))
			Expect(err).ToNot(HaveOccurred())

			signal, err := readSignal(reader, time.Second*3)
			Expect(err).ToNot(HaveOccurred())
			Expect(signal).Should(Equal(ErrorSignal))
		})

		Context("when writer is not writtable", func() {
			It("returns an error", func() {
				file, err := ioutil.TempFile("", "")
				Expect(err).ToNot(HaveOccurred())

				writer, err := os.Open(file.Name())
				Expect(err).ToNot(HaveOccurred())

				pipeSynchronizer.Writer = writer

				err = pipeSynchronizer.SignalError(errors.New("Bang Bang"))
				Expect(err).To(HaveOccurred())
			})
		})
	})
})

func readSignal(reader *os.File, timeout time.Duration) (containerizer.Signal, error) {
	signalQueue := make(chan containerizer.Signal)
	defer close(signalQueue)

	// Passing a file descriptor because Gingko cries for data races otherwise
	go func(readerFd uintptr, signalQueue chan containerizer.Signal) {
		var signal containerizer.Signal

		file := os.NewFile(readerFd, "/dev/hoooo")
		decoder := json.NewDecoder(file)
		defer file.Close()
		err := decoder.Decode(&signal)
		if err == nil {
			signalQueue <- signal
		}
	}(reader.Fd(), signalQueue)

	select {
	case signal := <-signalQueue:
		return signal, nil
	case <-time.After(timeout):
		return containerizer.Signal{}, errors.New("Reached timeout")
	}
}
