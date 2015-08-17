package rootfs_provider

import (
	"os"
	"path/filepath"

	"github.com/pivotal-golang/lager"
)

//go:generate counterfeiter -o fake_namespacer/fake_namespacer.go . Namespacer
type Namespacer interface {
	CacheKey() string
	Namespace(rootfsPath string) error
}

//go:generate counterfeiter -o fake_translator/fake_translator.go . Translator
type Translator interface {
	CacheKey() string
	Translate(path string, info os.FileInfo, err error) error
}

type UidNamespacer struct {
	Translator Translator
	Logger     lager.Logger
}

func (n *UidNamespacer) Namespace(rootfsPath string) error {
	log := n.Logger.Session("namespace-rootfs", lager.Data{
		"path": rootfsPath,
	})

	log.Info("namespace")

	if err := filepath.Walk(rootfsPath, n.Translator.Translate); err != nil {
		log.Error("walk-failed", err)
	}

	log.Info("namespaced")

	return nil
}

func (n *UidNamespacer) CacheKey() string {
	return n.Translator.CacheKey()
}
