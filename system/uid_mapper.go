package system

import (
	"fmt"
	"io/ioutil"
	"strconv"
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

func NewMappingList() (MappingList, error) {
	maxUid, err := MaxValidUid("/proc/self/uid_map", "/proc/self/gid_map")
	if err != nil {
		return nil, err
	}

	return MappingList{
		Mapping{
			FromID: 0,
			ToID:   maxUid,
			Size:   1,
		},
		Mapping{
			FromID: 1,
			ToID:   1,
			Size:   maxUid - 1,
		},
	}, nil
}

// The maximum valid uid is the minimum of the maximum uid and maximum gid as found in the specified files.
// This assumes that the container's uid and gid mappings will be identical.
func MaxValidUid(uidMapPath, gidMapPath string) (int, error) {
	maxUid, err := readMaxID(uidMapPath)
	if err != nil {
		return 0, err
	}

	maxGid, err := readMaxID(gidMapPath)
	if err != nil {
		return 0, err
	}

	if maxUid > maxGid {
		return maxGid, nil
	} else {
		return maxUid, nil
	}
}

func readMaxID(path string) (int, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("system: opening map file '%s': %s", path, err)
	}

	parts := strings.Fields(string(data))
	if len(parts) != 3 || parts[0] != "0" || parts[1] != "0" {
		return 0, fmt.Errorf("system: unsupported map file contents '%s'", string(data))
	}

	size, err := strconv.Atoi(parts[2])
	if err != nil {
		return 0, fmt.Errorf("system: invalid size in map file '%s': %s", path, err)
	}

	return size - 1, nil
}
