package measurements_test

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"code.cloudfoundry.org/garden"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const (
	iterations    = 0 // e.g. 50
	numStreams    = 0 // e.g. 128
	repeats       = 0 // e.g. 10
	streamSamples = 0 // e.g. 5
)

type byteCounterWriter struct {
	num *uint64
}

func (w *byteCounterWriter) Write(d []byte) (int, error) {
	atomic.AddUint64(w.num, uint64(len(d)))
	return len(d), nil
}

func (w *byteCounterWriter) Close() error {
	return nil
}

var _ = Describe("The Garden server", func() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	var container garden.Container
	var firstGoroutineCount uint64
	var debugAddr string

	BeforeEach(func() {
		firstGoroutineCount = 0
		debugAddr = fmt.Sprintf("0.0.0.0:%d", 15000+GinkgoParallelNode())
		client = startGarden("--debugAddr", debugAddr)

		var err error
		container, err = client.Create(garden.ContainerSpec{})
		Expect(err).ToNot(HaveOccurred())
	})

	getGoroutineCount := func(printIt ...bool) uint64 {
		resp, err := http.Get(fmt.Sprintf("http://%s/debug/pprof/goroutine?debug=1", debugAddr))
		Expect(err).ToNot(HaveOccurred())

		line, _, err := bufio.NewReader(resp.Body).ReadLine()
		Expect(err).ToNot(HaveOccurred())

		if len(printIt) > 0 && printIt[0] {
			io.Copy(os.Stdout, resp.Body)
		}

		words := strings.Split(string(line), " ")

		goroutineCount, err := strconv.ParseUint(words[len(words)-1], 10, 64)
		Expect(err).ToNot(HaveOccurred())

		return goroutineCount
	}

	Describe("repeatedly running processes", func() {
		Measure("does not leak goroutines", func(b Benchmarker) {
			for i := 1; i <= iterations; i++ {
				process, err := container.Run(garden.ProcessSpec{
					User: "alice",
					Path: "echo",
					Args: []string{"hi"},
				}, garden.ProcessIO{})
				Expect(err).ToNot(HaveOccurred())

				status, err := process.Wait()
				Expect(err).ToNot(HaveOccurred())
				Expect(status).To(Equal(0))

				if i == 1 {
					firstGoroutineCount = getGoroutineCount()
					b.RecordValue("first goroutine count", float64(firstGoroutineCount))
				}

				if i == iterations {
					lastGoroutineCount := getGoroutineCount()
					b.RecordValue("last goroutine count", float64(lastGoroutineCount))

					Expect(lastGoroutineCount).ToNot(BeNumerically(">", firstGoroutineCount+5))
				}
			}
		}, 1)
	})

	Describe("repeatedly attaching to a running process", func() {
		var processID string

		BeforeEach(func() {
			process, err := container.Run(garden.ProcessSpec{
				User: "alice",
				Path: "cat",
			}, garden.ProcessIO{})
			Expect(err).ToNot(HaveOccurred())

			processID = process.ID()
		})

		Measure("does not leak goroutines", func(b Benchmarker) {
			for i := 1; i <= iterations; i++ {
				stdoutR, stdoutW := io.Pipe()
				stdinR, stdinW := io.Pipe()

				_, err := container.Attach(processID, garden.ProcessIO{
					Stdin:  stdinR,
					Stdout: stdoutW,
				})
				Expect(err).ToNot(HaveOccurred())

				stdinData := fmt.Sprintf("hello %d", i)

				_, err = stdinW.Write([]byte(stdinData + "\n"))
				Expect(err).ToNot(HaveOccurred())

				var line []byte
				doneReading := make(chan struct{})
				go func() {
					line, _, err = bufio.NewReader(stdoutR).ReadLine()
					close(doneReading)
				}()

				Eventually(doneReading).Should(BeClosed())
				Expect(err).ToNot(HaveOccurred())
				Expect(string(line)).To(Equal(stdinData))

				stdinW.CloseWithError(errors.New("going away now"))

				if i == 1 {
					firstGoroutineCount = getGoroutineCount()
					b.RecordValue("first goroutine count", float64(firstGoroutineCount))
				}

				if i == iterations {
					lastGoroutineCount := getGoroutineCount()
					b.RecordValue("last goroutine count", float64(lastGoroutineCount))

					// TODO - we have a leak more.
					// Expect(lastGoroutineCount).ToNot(BeNumerically(">", firstGoroutineCount+5))
				}
			}
		}, 1)
	})

	Describe("streaming output from a chatty job", func() {
		streamCounts := []int{0}

		for i := 1; i <= numStreams; i *= 2 {
			streamCounts = append(streamCounts, i)
		}

		for _, streams := range streamCounts {
			Context(fmt.Sprintf("with %d streams", streams), func() {
				var started time.Time
				var receivedBytes uint64

				numToSpawn := streams

				BeforeEach(func() {
					atomic.StoreUint64(&receivedBytes, 0)
					started = time.Now()

					byteCounter := &byteCounterWriter{&receivedBytes}

					spawned := make(chan bool)

					for j := 0; j < numToSpawn; j++ {
						go func() {
							defer GinkgoRecover()

							_, err := container.Run(garden.ProcessSpec{
								User: "alice",
								Path: "cat",
								Args: []string{"/dev/zero"},
							}, garden.ProcessIO{
								Stdout: byteCounter,
							})
							Expect(err).ToNot(HaveOccurred())

							spawned <- true
						}()
					}

					for j := 0; j < numToSpawn; j++ {
						<-spawned
					}
				})

				AfterEach(func() {
					err := client.Destroy(container.Handle())
					Expect(err).ToNot(HaveOccurred())
				})

				Measure("it should not adversely affect the rest of the API", func(b Benchmarker) {
					var newContainer garden.Container

					b.Time("creating another container", func() {
						var err error

						newContainer, err = client.Create(garden.ContainerSpec{})
						Expect(err).ToNot(HaveOccurred())
					})

					for i := 0; i < repeats; i++ {
						b.Time("getting container info ("+strconv.Itoa(repeats)+"x)", func() {
							_, err := newContainer.Info()
							Expect(err).ToNot(HaveOccurred())
						})
					}

					for i := 0; i < repeats; i++ {
						b.Time("running a job ("+strconv.Itoa(repeats)+"x)", func() {
							process, err := newContainer.Run(garden.ProcessSpec{
								User: "alice", Path: "ls",
							}, garden.ProcessIO{})
							Expect(err).ToNot(HaveOccurred())

							Expect(process.Wait()).To(Equal(0))
						})
					}

					b.Time("destroying the container", func() {
						err := client.Destroy(newContainer.Handle())
						Expect(err).ToNot(HaveOccurred())
					})

					b.RecordValue(
						"received rate (bytes/second)",
						float64(atomic.LoadUint64(&receivedBytes))/float64(time.Since(started)/time.Second),
					)

					fmt.Println("total time:", time.Since(started))
				}, streamSamples)
			})
		}
	})
})
