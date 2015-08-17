package system

import (
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"
)

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
