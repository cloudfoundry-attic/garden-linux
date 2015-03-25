package process_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"os"
	"path/filepath"

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

	Describe("reading from file", func() {
		It("constructs the Env from a file", func() {
			cwd, err := os.Getwd()
			Ω(err).ShouldNot(HaveOccurred())
			pathToTestFile := filepath.Join(cwd, "test-assets", "sample")
			result, err := process.EnvFromFile(pathToTestFile)
			Ω(err).ShouldNot(HaveOccurred())

			Ω(result).Should(Equal(process.Env{
				"key1": "value1",
				"key2": "value2",
				"key3": "value=3",
			}))
		})

		Context("when reading a bad file path", func() {
			It("returns an error", func() {
				_, err := process.EnvFromFile("/nosuch")
				Ω(err).Should(MatchError(MatchRegexp("process: EnvFromFile: .* no such file .*")))
			})
		})

		Context("when the file is empty", func() {
			It("returns an empty env", func() {
				cwd, err := os.Getwd()
				Ω(err).ShouldNot(HaveOccurred())
				pathToTestFile := filepath.Join(cwd, "test-assets", "empty")
				result, err := process.EnvFromFile(pathToTestFile)
				Ω(err).ShouldNot(HaveOccurred())
				Ω(result).Should(Equal(process.Env{}))
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

		It("supports using the '=' character in environment variable values", func() {
			env, err := process.NewEnv([]string{
				"KEY1==",
				"KEY2=atend=",
				"KEY3==atbeginning",
				"KEY4=in=middle",
				"KEY5=multiple=equal=signs",
			})
			Ω(err).ShouldNot(HaveOccurred())
			Ω(env.Array()).Should(ConsistOf(
				"KEY1==",
				"KEY2=atend=",
				"KEY3==atbeginning",
				"KEY4=in=middle",
				"KEY5=multiple=equal=signs",
			))
		})

		Context("when the array is empty", func() {
			It("returns an empty env", func() {
				env, err := process.NewEnv([]string{})
				Ω(err).ShouldNot(HaveOccurred())

				Ω(env).Should(Equal(process.Env{}))
			})
		})

		Context("when the array is malformed", func() {
			It("returns an error when the array contains an empty string", func() {
				env, err := process.NewEnv([]string{""})

				Ω(err).Should(MatchError("process: malformed environment: empty string"))
				Ω(env).Should(BeNil())
			})

			It("returns an error when the array contains an element with an empty key", func() {
				env, err := process.NewEnv([]string{"=value"})

				Ω(err).Should(MatchError(`process: malformed environment: empty key: "=value"`))
				Ω(env).Should(BeNil())
			})

			It("returns an error when the array contains an element without an equals sign", func() {
				env, err := process.NewEnv([]string{"x"})

				Ω(err).Should(MatchError(`process: malformed environment: invalid format (not key=value): "x"`))
				Ω(env).Should(BeNil())
			})
		})
	})

	It("produces a string representation", func() {
		Ω(process.Env{"a": "b"}.String()).Should(Equal(`process.Env{"a":"b"}`))
	})
})
