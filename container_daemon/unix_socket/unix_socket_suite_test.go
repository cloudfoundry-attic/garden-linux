package unix_socket_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestUnixSocket(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UnixSocket Suite")
}
