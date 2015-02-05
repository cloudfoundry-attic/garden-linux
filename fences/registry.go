package fences

import (
	"encoding/json"
	"errors"
	"flag"

	"github.com/cloudfoundry-incubator/garden"
	"github.com/cloudfoundry-incubator/garden-linux/old/sysconfig"
	"github.com/cloudfoundry-incubator/garden-linux/process"
)

var ErrNoFencesRegistered = errors.New("no fences have been registered")

type FlagFunc func(*flag.FlagSet) error

type FlagProcessors struct {
	initFuncs []FlagFunc
	mainFuncs []func(*BuilderRegistry) error
}

type BuilderRegistry struct {
	builders []Builder
}

var flagProcessors *FlagProcessors = &FlagProcessors{}

type Builder interface {
	Build(spec string, sysconfig *sysconfig.Config, containerID string) (Fence, error)
	Rebuild(*json.RawMessage) (Fence, error)
	Capacity() int
}

type Fence interface {
	json.Marshaler
	ConfigureProcess(process.Env) error
	Dismantle() error
	Info(*garden.ContainerInfo)
	String() string
}

type Process interface {
	ConfigureEnv(key, value string)
}

func Register(init FlagFunc, main func(*BuilderRegistry) error) {
	flagProcessors.Register(init, main)
}

func Main(fs *flag.FlagSet, args []string) (*BuilderRegistry, error) {

	return flagProcessors.Main(fs, args)
}

func (r *FlagProcessors) Register(init FlagFunc, main func(*BuilderRegistry) error) {
	r.initFuncs = append(r.initFuncs, init)
	r.mainFuncs = append(r.mainFuncs, main)
}

func (r *FlagProcessors) Main(fs *flag.FlagSet, args []string) (*BuilderRegistry, error) {
	for _, f := range r.initFuncs {
		f(fs)
	}

	fs.Parse(args)

	builders := &BuilderRegistry{}
	for _, m := range r.mainFuncs {
		if err := m(builders); err != nil {
			return nil, err
		}
	}

	return builders, nil
}

func (r *BuilderRegistry) Register(a Builder) {
	r.builders = append(r.builders, a)

	if len(r.builders) > 1 {
		panic("multiple builders not implemented")
	}
}

func (r *BuilderRegistry) Capacity() int {
	for _, r := range r.builders {
		return r.Capacity()
	}

	return 0
}

func (r *BuilderRegistry) Build(spec string, sysconfig *sysconfig.Config, containerID string) (Fence, error) {
	for _, r := range r.builders {
		return r.Build(spec, sysconfig, containerID)
	}

	return nil, ErrNoFencesRegistered
}

func (r *BuilderRegistry) Rebuild(rm *json.RawMessage) (Fence, error) {
	for _, r := range r.builders {
		return r.Rebuild(rm)
	}

	return nil, ErrNoFencesRegistered
}
