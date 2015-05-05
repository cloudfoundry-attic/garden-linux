package rootfs_provider

var DefaultUIDMap MappingList = MappingList{{
	FromID: 0,
	ToID:   600000, // todo: pick a range of IDS high enough not to conflict with anything, e.g. 65534+
	Size:   599999,
}}

var DefaultGIDMap = DefaultUIDMap

type Mapping struct {
	FromID int
	ToID   int
	Size   int
}

type MappingList []Mapping

func (m MappingList) Map(id int) int {
	for _, m := range m {
		if delta := id - m.FromID; delta < m.Size {
			return m.ToID + delta
		}
	}

	return id
}
