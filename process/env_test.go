package process_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"os"
	"path/filepath"

	"code.cloudfoundry.org/garden-linux/process"
)

var _ = Describe("Environment", func() {
	Context("with an empty environment", func() {
		It("turns into an empty array", func() {
			env := process.Env{}
			Expect(env.Array()).To(BeEmpty())
		})
	})

	Context("with a non empty environment", func() {
		It("converts the environment into the corresponding array", func() {
			env := process.Env{
				"HOME": "/home/alice",
				"USER": "alice",
			}
			Expect(env.Array()).To(ConsistOf(
				"HOME=/home/alice",
				"USER=alice",
			))
		})

		It("sorts the keys into a predictable order", func() {
			envForwards := process.Env{
				"HOME": "/home/alice",
				"USER": "alice",
			}
			envBackwards := process.Env{
				"USER": "alice",
				"HOME": "/home/alice",
			}

			Expect(envForwards.Array()).To(Equal(envBackwards.Array()))
		})

		Describe("merging in a second environment", func() {
			It("adds the new environment to the old one", func() {
				old := process.Env{
					"HOME": "/home/alice",
				}
				extra := process.Env{
					"USER": "alice",
				}

				merged := old.Merge(extra)
				Expect(merged.Array()).To(ConsistOf(
					"HOME=/home/alice",
					"USER=alice",
				))
			})

			It("merges the new environment into the old one (new values win)", func() {
				old := process.Env{
					"USER": "root",
				}
				extra := process.Env{
					"USER": "alice",
				}

				merged := old.Merge(extra)
				Expect(merged.Array()).To(ConsistOf(
					"USER=alice",
				))
			})
		})
	})

	Describe("reading from file", func() {
		It("constructs the Env from a file", func() {
			cwd, err := os.Getwd()
			Expect(err).ToNot(HaveOccurred())
			pathToTestFile := filepath.Join(cwd, "test-assets", "sample")
			result, err := process.EnvFromFile(pathToTestFile)
			Expect(err).ToNot(HaveOccurred())

			Expect(result).To(Equal(process.Env{
				"key1": "value1",
				"key2": "value2",
				"key3": "value=3",
			}))
		})

		Context("when reading a bad file path", func() {
			It("returns an error", func() {
				_, err := process.EnvFromFile("/nosuch")
				Expect(err).To(MatchError(MatchRegexp("process: EnvFromFile: .* no such file .*")))
			})
		})

		Context("when the file is empty", func() {
			It("returns an empty env", func() {
				cwd, err := os.Getwd()
				Expect(err).ToNot(HaveOccurred())
				pathToTestFile := filepath.Join(cwd, "test-assets", "empty")
				result, err := process.EnvFromFile(pathToTestFile)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(process.Env{}))
			})
		})
	})

	Context("when using the constructor", func() {
		It("can be constructed from an array", func() {
			env, err := process.NewEnv([]string{
				"HOME=/home/alice",
				"USER=alice",
			})
			Expect(err).ToNot(HaveOccurred())

			Expect(env.Array()).To(ConsistOf(
				"HOME=/home/alice",
				"USER=alice",
			))
		})

		It("removes duplicate entries (last one wins)", func() {
			env, err := process.NewEnv([]string{
				"HOME=/home/wrong",
				"HOME=/home/alice",
			})
			Expect(err).ToNot(HaveOccurred())

			Expect(env.Array()).To(ConsistOf(
				"HOME=/home/alice",
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
			Expect(err).ToNot(HaveOccurred())
			Expect(env.Array()).To(ConsistOf(
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
				Expect(err).ToNot(HaveOccurred())

				Expect(env).To(Equal(process.Env{}))
			})
		})

		Context("when the array is malformed", func() {
			It("returns an error when the array contains an empty string", func() {
				env, err := process.NewEnv([]string{""})

				Expect(err).To(MatchError("process: malformed environment: empty string"))
				Expect(env).To(BeNil())
			})

			It("returns an error when the array contains an element with an empty key", func() {
				env, err := process.NewEnv([]string{"=value"})

				Expect(err).To(MatchError(`process: malformed environment: empty key: "=value"`))
				Expect(env).To(BeNil())
			})

			It("returns an error when the array contains an element without an equals sign", func() {
				env, err := process.NewEnv([]string{"x"})

				Expect(err).To(MatchError(`process: malformed environment: invalid format (not key=value): "x"`))
				Expect(env).To(BeNil())
			})
		})
	})

	It("produces a string representation", func() {
		Expect(process.Env{"a": "b"}.String()).To(Equal(`process.Env{"a":"b"}`))
	})
})
