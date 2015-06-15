package legacy_aufs_remover

import "github.com/pivotal-golang/lager"

//go:generate counterfeiter -o fake_unmounter/FakeUnmounter.go . Unmounter
type Unmounter interface {
	Unmount(dir string) error
}

type Remover struct {
	Unmounter Unmounter
	DepotDir  string
}

func (r *Remover) CleanupRootFS(logger lager.Logger, id string) error {
	return r.Unmounter.Unmount(path)
}
