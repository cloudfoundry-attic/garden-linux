package lifecycle_test

import (
	"fmt"
	"strings"

	warden "github.com/cloudfoundry-incubator/garden/protocol"
	"github.com/cloudfoundry-incubator/gordon"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Placing limits on containers", func() {
	Describe("denying access to network ranges", func() {
		var (
			blockedListenerHandle string
			blockedListenerIP     string

			unblockedListenerHandle string
			unblockedListenerIP     string

			allowedListenerHandle string
			allowedListenerIP     string

			senderHandle string
		)

		BeforeEach(func() {
			var err error

			// create a listener to which we deny network access
			res, err := client.Create(nil)
			Expect(err).ToNot(HaveOccurred())
			blockedListenerHandle = res.GetHandle()
			infoRes, err := client.Info(blockedListenerHandle)
			Expect(err).ToNot(HaveOccurred())
			blockedListenerIP = infoRes.GetContainerIp()

			// create a listener to which we do not deny access
			res, err = client.Create(nil)
			Expect(err).ToNot(HaveOccurred())
			unblockedListenerHandle = res.GetHandle()
			infoRes, err = client.Info(unblockedListenerHandle)
			Expect(err).ToNot(HaveOccurred())
			unblockedListenerIP = infoRes.GetContainerIp()

			// create a listener to which we exclicitly allow access
			res, err = client.Create(nil)
			Expect(err).ToNot(HaveOccurred())
			allowedListenerHandle = res.GetHandle()
			infoRes, err = client.Info(allowedListenerHandle)
			Expect(err).ToNot(HaveOccurred())
			allowedListenerIP = infoRes.GetContainerIp()

			runner.Stop()
			runner.Start(
				"-denyNetworks", strings.Join([]string{
					blockedListenerIP + "/32",
					allowedListenerIP + "/32",
				}, ","),
				"-allowNetworks", allowedListenerIP+"/32",
			)

			// create a container with the new deny network configuration
			res, err = client.Create(nil)
			Expect(err).ToNot(HaveOccurred())
			senderHandle = res.GetHandle()
		})

		AfterEach(func() {
			_, err := client.Destroy(senderHandle)
			Expect(err).ToNot(HaveOccurred())

			_, err = client.Destroy(blockedListenerHandle)
			Expect(err).ToNot(HaveOccurred())

			_, err = client.Destroy(unblockedListenerHandle)
			Expect(err).ToNot(HaveOccurred())

			_, err = client.Destroy(allowedListenerHandle)
			Expect(err).ToNot(HaveOccurred())
		})

		expectStreamToExitWith := func(stream <-chan *warden.ProcessPayload, status int) {
			for chunk := range stream {
				if chunk.ExitStatus != nil {
					Expect(chunk.GetExitStatus()).To(Equal(uint32(status)))
				}
			}
		}

		runWithHandle := func(handle, script string) <-chan *warden.ProcessPayload {
			_, stream, err := client.Run(handle, script, gordon.ResourceLimits{}, nil)
			Expect(err).ToNot(HaveOccurred())
			return stream
		}

		It("makes that block of ip addresses inaccessible to the container", func() {
			runWithHandle(blockedListenerHandle, "nc -l 12345")
			runWithHandle(unblockedListenerHandle, "nc -l 12345")
			runWithHandle(allowedListenerHandle, "nc -l 12345")

			senderStream := runWithHandle(senderHandle, fmt.Sprintf("echo hello | nc -w 1 %s 12345", blockedListenerIP))
			expectStreamToExitWith(senderStream, 1)

			senderStream = runWithHandle(senderHandle, fmt.Sprintf("echo hello | nc -w 1 %s 12345", unblockedListenerIP))
			expectStreamToExitWith(senderStream, 0)

			senderStream = runWithHandle(senderHandle, fmt.Sprintf("echo hello | nc -w 1 %s 12345", allowedListenerIP))
			expectStreamToExitWith(senderStream, 0)
		})
	})
})
