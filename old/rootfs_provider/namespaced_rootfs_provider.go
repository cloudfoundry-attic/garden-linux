package rootfs_provider

import (
	"path/filepath"

	"github.com/pivotal-golang/lager"
)

//go:generate counterfeiter -o fake_namespacer/fake_namespacer.go . Namespacer
type Namespacer interface {
	Namespace(rootfsPath string) error
}

type UidNamespacer struct {
	Translator filepath.WalkFunc
	Logger     lager.Logger
}

func (n *UidNamespacer) Namespace(rootfsPath string) error {
	log := n.Logger.Session("namespace-rootfs", lager.Data{
		"path": rootfsPath,
	})

	log.Info("namespace")

	if err := filepath.Walk(rootfsPath, n.Translator); err != nil {
		log.Error("walk-failed", err)
	}

	log.Info("namespaced")

	return nil
}
