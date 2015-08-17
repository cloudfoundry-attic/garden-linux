package rootfs_provider

import (
	"fmt"
	"os"
)

type UidTranslator struct {
	uidMappings StringMapper
	gidMappings StringMapper

	getuidgid func(os.FileInfo) (int, int, error)
	chown     func(path string, uid, gid int) error
}

type Mapper interface {
	Map(id int) int
}

type StringMapper interface {
	fmt.Stringer
	Mapper
}

func NewUidTranslator(uidMappings StringMapper, gidMappings StringMapper) *UidTranslator {
	return &UidTranslator{
		uidMappings: uidMappings,
		gidMappings: gidMappings,

		getuidgid: getuidgid,
		chown:     os.Lchown,
	}
}

func (u UidTranslator) Translate(path string, info os.FileInfo, err error) error {
	uid, gid, _ := u.getuidgid(info)
	touid, togid := u.uidMappings.Map(uid), u.gidMappings.Map(gid)

	if touid != uid || togid != gid {
		u.chown(path, touid, togid)
	}

	return nil
}

func (u UidTranslator) CacheKey() string {
	return fmt.Sprintf("%s+%s", u.uidMappings.String(), u.gidMappings.String())
}
