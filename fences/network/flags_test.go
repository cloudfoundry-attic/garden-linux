package network

import (
	"flag"
	"fmt"
	"io/ioutil"
	"math"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Flags", func() {
	var (
		args   []string
		flags  *flag.FlagSet
		config *Config
	)

	JustBeforeEach(func() {
		flags = flag.NewFlagSet("test", flag.ContinueOnError)
		flags.SetOutput(ioutil.Discard)

		config = &Config{}
		config.Init(flags)
	})

	Describe("-networkPool", func() {
		Context("when it is not set", func() {
			It("uses the default", func() {
				flags.Parse(args)
				Ω(config.Network.IPNet.String()).Should(Equal(DefaultNetworkPool))
			})
		})

		Context("when it is set", func() {
			Context("and when it is a valid CIDR address", func() {
				BeforeEach(func() {
					args = []string{"-networkPool", "1.2.3.0/29"}
				})

				It("parses succesfully", func() {
					flags.Parse(args)
					Ω(config.Network.IPNet.String()).Should(Equal("1.2.3.0/29"))
				})
			})

			Context("and when it is invalid", func() {
				BeforeEach(func() {
					args = []string{"-networkPool", "invalid"}
				})

				It("should error, naming the right flag", func() {
					err := flags.Parse(args)
					Ω(err).Should(HaveOccurred())
					Ω(err.Error()).Should(MatchRegexp("-networkPool"))
				})
			})
		})
	})

	Describe("-mtu", func() {
		Context("when it is not set", func() {
			It("uses the default", func() {
				flags.Parse(args)
				Ω(config.Mtu).Should(BeEquivalentTo(DefaultMTUSize))
			})
		})

		Context("when it is set", func() {
			Context("and when it is a valid mtu", func() {
				BeforeEach(func() {
					args = []string{"-mtu", "123"}
				})

				It("parses succesfully", func() {
					flags.Parse(args)
					Ω(config.Mtu).Should(BeEquivalentTo(123))
				})
			})

			Context("and when it is too large", func() {
				BeforeEach(func() {
					args = []string{"-mtu", fmt.Sprintf("%d", math.MaxUint32+1)}
				})

				It("should error, naming the right flag", func() {
					err := flags.Parse(args)
					Ω(err).Should(HaveOccurred())
					Ω(err.Error()).Should(MatchRegexp("-mtu"))
				})
			})

			Context("and when it is invalid", func() {
				BeforeEach(func() {
					args = []string{"-mtu", "invalid"}
				})

				It("should error, naming the right flag", func() {
					err := flags.Parse(args)
					Ω(err).Should(HaveOccurred())
					Ω(err.Error()).Should(MatchRegexp("-mtu"))
				})
			})
		})
	})
})
