package fake_cnet_persistor

import "github.com/cloudfoundry-incubator/garden-linux/network/cnet"

type FakeCNPersistor struct {
	RecoverResult cnet.ContainerNetwork
	PersistError  error
	RebuildError  error
}

func New() *FakeCNPersistor {
	return &FakeCNPersistor{}
}

func (fcnp *FakeCNPersistor) Persist(cn cnet.ContainerNetwork, path string) error {
	return fcnp.PersistError
}

func (fcnp *FakeCNPersistor) Recover(path string) (cnet.ContainerNetwork, error) {
	if fcnp.RebuildError != nil {
		return nil, fcnp.RebuildError
	}
	return fcnp.RecoverResult, nil
}
