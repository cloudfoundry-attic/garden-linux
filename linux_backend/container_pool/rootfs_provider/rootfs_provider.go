package rootfs_provider

type RootFSProvider interface {
	ProvideRootFS(id, name string) (mountpoint string, err error)
	CleanupRootFS(id string) error
}
