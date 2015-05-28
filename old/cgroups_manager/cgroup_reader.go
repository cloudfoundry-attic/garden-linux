package cgroups_manager

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
)

type LinuxCgroupReader struct {
	Path     string
	openFile *os.File
}

func (c *LinuxCgroupReader) CgroupNode(subsystem string) (string, error) {
	if c.openFile == nil {
		var err error

		c.openFile, err = os.Open(c.Path)
		if err != nil {
			return "", fmt.Errorf("cgroups_manager: opening file: %s", err)
		}
	}

	_, err := c.openFile.Seek(0, 0)
	if err != nil {
		return "", fmt.Errorf("cgroups_manager: seeking start of file: %s", err)
	}

	contents, err := ioutil.ReadAll(c.openFile)
	if err != nil {
		return "", fmt.Errorf("cgroups_manager: reading file: %s", err)
	}

	line := findCgroupEntry(string(contents), subsystem)
	if line == "" {
		return "", fmt.Errorf("cgroups_manager: requested subsystem %s does not exist", subsystem)
	}

	cgroupEntryColumns := strings.Split(line, ":")
	if len(cgroupEntryColumns) != 3 {
		return "", fmt.Errorf("cgroups_manager: cgroup file malformed: %s", string(contents))
	}

	return cgroupEntryColumns[2], nil
}

func findCgroupEntry(contents, subsystem string) string {
	for _, line := range strings.Split(contents, "\n") {
		if strings.Contains(line, fmt.Sprintf("%s:", subsystem)) {
			return line
		}
	}

	return ""
}
