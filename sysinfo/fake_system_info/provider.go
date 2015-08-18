package fake_system_info

type FakeProvider struct {
	TotalMemoryResult uint64
	TotalMemoryError  error

	TotalDiskResult uint64
	TotalDiskError  error
}

func NewFakeProvider() *FakeProvider {
	return &FakeProvider{}
}

func (provider *FakeProvider) TotalMemory() (uint64, error) {
	if provider.TotalMemoryError != nil {
		return 0, provider.TotalMemoryError
	}

	return provider.TotalMemoryResult, nil
}

func (provider *FakeProvider) TotalDisk() (uint64, error) {
	if provider.TotalDiskError != nil {
		return 0, provider.TotalDiskError
	}

	return provider.TotalDiskResult, nil
}
