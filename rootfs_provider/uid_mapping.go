package rootfs_provider

import (
	"fmt"
	"strings"
)

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

func (m MappingList) String() string {
	if len(m) == 0 {
		return "empty"
	}

	var parts []string
	for _, entry := range m {
		parts = append(parts, fmt.Sprintf("%d-%d-%d", entry.FromID, entry.ToID, entry.Size))
	}

	return strings.Join(parts, ",")
}
