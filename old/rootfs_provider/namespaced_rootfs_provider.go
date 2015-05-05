package rootfs_provider

import (
	"fmt"
	"os/exec"
	"path/filepath"

	"github.com/pivotal-golang/lager"
)

//go:generate counterfeiter -o fake_namespacer/fake_namespacer.go . Namespacer
type Namespacer interface {
	Namespace(src, dest string) error
}

//go:generate counterfeiter -o fake_copier/fake_copier.go . Copier
type Copier interface {
	Copy(src, dest string) error
}

type UidNamespacer struct {
	Copier     Copier
	Translator filepath.WalkFunc
	Logger     lager.Logger
}

func (n *UidNamespacer) Namespace(src, dest string) error {
	log := n.Logger.Session("namespace-rootfs", lager.Data{
		"src":  src,
		"dest": dest,
	})

	log.Info("namespace")

	err := n.Copier.Copy(src, dest)
	if err != nil {
		return err
	}

	if err := filepath.Walk(dest, n.Translator); err != nil {
		log.Error("walk-failed", err)
	}

	log.Info("namespaced")

	return nil
}

type ShellOutCp struct {
	WorkDir string
}

func (s ShellOutCp) Copy(src, dest string) error {
	cmd := exec.Command("sh", "-c", fmt.Sprintf("cp -a %s/* %s/", src, dest))
	if err := cmd.Run(); err != nil {
		return err
	}

	return nil
}
