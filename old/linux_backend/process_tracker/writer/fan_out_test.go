package writer_test

import (
	"github.com/cloudfoundry-incubator/garden-linux/old/linux_backend/process_tracker/writer"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
    "github.com/onsi/gomega"
    "errors")

var _ = Describe("FanOut", func() {
    var fanOut writer.FanOut
    var fWriter *fakeWriter
    var testBytes []byte

    BeforeEach(func(){
        fanOut = writer.NewFanOut()
        fWriter = &fakeWriter{
            nWriteReturn : 10,
        }
        testBytes = []byte{12}
    })

    It("writes data to a sink", func(){
        fanOut.AddSink(fWriter)
        n, err := fanOut.Write(testBytes)

        Ω(err).ShouldNot(gomega.HaveOccurred())
        Ω(n).Should(Equal(1))

        Ω(fWriter.writeArgument()).Should(Equal(testBytes))
        Ω(fWriter.writeCalls()).Should(Equal(1))
    })

    It("ignores errors when writing to the sink", func(){
        fWriter.errWriteReturn = errors.New("write error")
        fanOut.AddSink(fWriter)
        n, err := fanOut.Write(testBytes)

        Ω(err).ShouldNot(gomega.HaveOccurred())
        Ω(n).Should(Equal(1))
    })

    It("writes data to two sinks", func(){
        fWriter2 := &fakeWriter{
            nWriteReturn : 10,
        }
        fanOut.AddSink(fWriter2)
        fanOut.AddSink(fWriter)
        n, err := fanOut.Write(testBytes)

        Ω(err).ShouldNot(gomega.HaveOccurred())
        Ω(n).Should(Equal(1))

        Ω(fWriter.writeArgument()).Should(Equal(testBytes))
        Ω(fWriter.writeCalls()).Should(Equal(1))

        Ω(fWriter2.writeArgument()).Should(Equal(testBytes))
        Ω(fWriter2.writeCalls()).Should(Equal(1))
    })

    It("copes when there are no sinks", func(){
        n, err := fanOut.Write(testBytes)

        Ω(err).ShouldNot(gomega.HaveOccurred())
        Ω(n).Should(Equal(1))
    })
})
