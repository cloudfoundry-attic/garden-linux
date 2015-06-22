package rootfs_provider

import "os"

type UidTranslator struct {
	uidMappings Mapper
	gidMappings Mapper

	getuidgid func(os.FileInfo) (int, int, error)
	chown     func(path string, uid, gid int) error
}

type Mapper interface {
	Map(id int) int
}

func NewUidTranslator(uidMappings Mapper, gidMappings Mapper) *UidTranslator {
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
