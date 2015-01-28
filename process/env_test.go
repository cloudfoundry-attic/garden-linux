package process_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/cloudfoundry-incubator/garden-linux/process"
)

var _ = Describe("Environment", func() {
	Context("with an empty environment", func() {
		It("turns into an empty array", func() {
			env := process.Env{}
			Ω(env.Array()).Should(BeEmpty())
		})
	})

	Context("with a non empty environment", func() {
		It("converts the environment into the corresponding array", func() {
			env := process.Env{
				"HOME": "/home/vcap",
				"USER": "vcap",
			}
			Ω(env.Array()).Should(ConsistOf(
				"HOME=/home/vcap",
				"USER=vcap",
			))
		})

		It("sorts the keys into a predictable order", func() {
			envForwards := process.Env{
				"HOME": "/home/vcap",
				"USER": "vcap",
			}
			envBackwards := process.Env{
				"USER": "vcap",
				"HOME": "/home/vcap",
			}

			Ω(envForwards.Array()).To(Equal(envBackwards.Array()))
		})

		Describe("merging in a second environment", func() {
			It("adds the new environment to the old one", func() {
				old := process.Env{
					"HOME": "/home/vcap",
				}
				extra := process.Env{
					"USER": "vcap",
				}

				merged := old.Merge(extra)
				Ω(merged.Array()).Should(ConsistOf(
					"HOME=/home/vcap",
					"USER=vcap",
				))
			})

			It("merges the new environment into the old one (new values win)", func() {
				old := process.Env{
					"USER": "root",
				}
				extra := process.Env{
					"USER": "vcap",
				}

				merged := old.Merge(extra)
				Ω(merged.Array()).Should(ConsistOf(
					"USER=vcap",
				))
			})
		})
	})

	Context("when using the constructor", func() {
		It("can be constructed from an array", func() {
			env, err := process.NewEnv([]string{
				"HOME=/home/vcap",
				"USER=vcap",
			})
			Ω(err).ShouldNot(HaveOccurred())

			Ω(env.Array()).Should(ConsistOf(
				"HOME=/home/vcap",
				"USER=vcap",
			))
		})

		It("removes duplicate entries (last one wins)", func() {
			env, err := process.NewEnv([]string{
				"HOME=/home/wrong",
				"HOME=/home/vcap",
			})
			Ω(err).ShouldNot(HaveOccurred())

			Ω(env.Array()).Should(ConsistOf(
				"HOME=/home/vcap",
			))
		})

		Context("when the array is malformed", func() {
			It("returns an error when the array contains an empty string", func() {
				env, err := process.NewEnv([]string{""})

				Ω(err).Should(MatchError("malformed environment: empty string"))
				Ω(env).Should(BeNil())
			})

			It("returns an error when the array contains an element with an empty key", func() {
				env, err := process.NewEnv([]string{"=value"})

				Ω(err).Should(MatchError(`malformed environment: empty key: "=value"`))
				Ω(env).Should(BeNil())
			})

			It("returns an error when the array contains an element with too many equals signs", func() {
				env, err := process.NewEnv([]string{"key=value="})

				Ω(err).Should(MatchError(`malformed environment: invalid format (not key=value): "key=value="`))
				Ω(env).Should(BeNil())
			})

			It("returns an error when the array contains an element without an equals sign", func() {
				env, err := process.NewEnv([]string{"x"})

				Ω(err).Should(MatchError(`malformed environment: invalid format (not key=value): "x"`))
				Ω(env).Should(BeNil())
			})
		})
	})

	It("produces a string representation", func() {
		Ω(process.Env{"a": "b"}.String()).Should(Equal(`process.Env{"a":"b"}`))
	})
})
