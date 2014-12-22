package network

import (
	"fmt"

	"github.com/cloudfoundry-incubator/garden/api"
)

type FilterFactory interface {
	fmt.Stringer
	Create(id string) Filter
}

type Filter interface {
	NetOut(network string, port uint32, protocol api.Protocol) error
}

type filterFactory struct {
	instancePrefix string
}

type filter struct {
	instanceChain string
}

func NewFilterFactory(tag string) FilterFactory {
	return &filterFactory{instancePrefix: fmt.Sprintf("w-%s-instance-", tag)}
}

func (ff *filterFactory) Create(id string) Filter {
	return &filter{instanceChain: ff.instancePrefix + id}
}

func (ff *filterFactory) String() string {
	return fmt.Sprintf("%#v", ff)
}

func (fltr *filter) NetOut(network string, port uint32, protocol api.Protocol) error {
	return nil
}
