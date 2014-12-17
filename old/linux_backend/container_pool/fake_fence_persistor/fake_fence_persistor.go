package fake_fence_persistor

import "github.com/cloudfoundry-incubator/garden-linux/fences"

type FakeFencePersistor struct {
	RecoverResult fences.Fence
	PersistError  error
	RebuildError  error
}

func New() *FakeFencePersistor {
	return &FakeFencePersistor{}
}

func (ffp *FakeFencePersistor) Persist(fence fences.Fence, path string) error {
	return ffp.PersistError
}

func (ffp *FakeFencePersistor) Recover(path string) (fences.Fence, error) {
	if ffp.RebuildError != nil {
		return nil, ffp.RebuildError
	}
	return ffp.RecoverResult, nil
}
