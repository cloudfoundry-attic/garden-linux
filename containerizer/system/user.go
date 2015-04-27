package system

import (
	"fmt"
	osuser "os/user"

	"github.com/docker/libcontainer/user"
)

//go:generate counterfeiter -o fake_user/fake_user.go . User
type User interface {
	Lookup(name string) (*osuser.User, error)
}

type LibContainerUser struct{}

func (LibContainerUser) Lookup(name string) (*osuser.User, error) {
	u, err := user.LookupUser(name)
	return &osuser.User{
		Uid: fmt.Sprintf("%d", u.Uid),
		Gid: fmt.Sprintf("%d", u.Gid),
	}, err
}
