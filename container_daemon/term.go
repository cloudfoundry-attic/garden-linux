package container_daemon

import (
	"github.com/docker/docker/pkg/term"
)

type TermPkg struct{}

func (TermPkg) GetWinsize(fd uintptr) (*term.Winsize, error) {
	return term.GetWinsize(fd)
}

func (TermPkg) SetWinsize(fd uintptr, size *term.Winsize) error {
	return term.SetWinsize(fd, size)
}

func (TermPkg) SetRawTerminal(fd uintptr) (*term.State, error) {
	return term.SetRawTerminal(fd)
}

func (TermPkg) RestoreTerminal(fd uintptr, state *term.State) error {
	return term.RestoreTerminal(fd, state)
}
