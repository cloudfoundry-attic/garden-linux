package container_daemon

import (
	"fmt"
	osuser "os/user"

	"github.com/docker/libcontainer/user"
)

type LibContainerUser struct{}

func (LibContainerUser) Lookup(name string) (*osuser.User, error) {
	u, err := user.LookupUser(name)
	return &osuser.User{
		Uid:     fmt.Sprintf("%d", u.Uid),
		Gid:     fmt.Sprintf("%d", u.Gid),
		HomeDir: u.Home,
	}, err
}
