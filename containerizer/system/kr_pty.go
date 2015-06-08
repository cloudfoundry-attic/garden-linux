package system

import (
	"os"

	"github.com/kr/pty"
)

type krPty int

var KrPty krPty = 0

func (krPty) Open() (*os.File, *os.File, error) {
	return pty.Open()
}
