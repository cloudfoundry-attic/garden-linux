package system

import (
	"github.com/docker/docker/pkg/term"
)

// wraps docker/docker/pkg/term for mockability
//go:generate counterfeiter -o fake_term/fake_term.go . Term
type Term interface {
	GetWinsize(fd uintptr) (*term.Winsize, error)
	SetWinsize(fd uintptr, size *term.Winsize) error

	SetRawTerminal(fd uintptr) (*term.State, error)
	RestoreTerminal(fd uintptr, state *term.State) error
}

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
