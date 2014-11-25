package uid_pool

type UIDBlockPool interface {
	Acquire() (uint32, error)
	Remove(uint32) error
	Release(uint32)
	InitialSize() int
	BlockSize() uint32
}
