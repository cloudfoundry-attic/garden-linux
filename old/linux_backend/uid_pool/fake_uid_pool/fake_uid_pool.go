package fake_uid_pool

type FakeUIDPool struct {
	nextUID   uint32
	blockSize uint32

	InitialPoolSize int

	AcquireError error
	RemoveError  error

	Acquired []uint32
	Released []uint32
	Removed  []uint32
}

func New(start uint32, blockSize uint32) *FakeUIDPool {
	return &FakeUIDPool{
		nextUID:   start,
		blockSize: blockSize,
	}
}

func (p *FakeUIDPool) InitialSize() int {
	return p.InitialPoolSize
}

func (p *FakeUIDPool) BlockSize() uint32 {
	return p.blockSize
}

func (p *FakeUIDPool) Acquire() (uint32, error) {
	if p.AcquireError != nil {
		return 0, p.AcquireError
	}

	uid := p.nextUID
	p.nextUID = p.nextUID + p.blockSize

	return uid, nil
}

func (p *FakeUIDPool) Remove(uid uint32) error {
	if p.RemoveError != nil {
		return p.RemoveError
	}

	p.Removed = append(p.Removed, uid)

	return nil
}

func (p *FakeUIDPool) Release(uid uint32) {
	p.Released = append(p.Released, uid)
}
