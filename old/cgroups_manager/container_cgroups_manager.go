package cgroups_manager

import (
	"fmt"
	"io/ioutil"
	"path"
	"strings"
)

type ContainerCgroupsManager struct {
	cgroupsPath  string
	containerID  string
	cgroupReader CgroupReader
}

//go:generate counterfeiter -o fake_cgroup_reader/FakeCgroupReader.go . CgroupReader
type CgroupReader interface {
	CgroupNode(subsytem string) (string, error)
}

func New(cgroupsPath, containerID string, cgroupReader CgroupReader) *ContainerCgroupsManager {
	return &ContainerCgroupsManager{cgroupsPath, containerID, cgroupReader}
}

func (m *ContainerCgroupsManager) Set(subsystem, name, value string) error {
	cgroupPath, err := m.SubsystemPath(subsystem)
	if err != nil {
		return fmt.Errorf("cgroups_manager: set: %s", err)
	}
	return ioutil.WriteFile(path.Join(cgroupPath, name), []byte(value), 0644)
}

func (m *ContainerCgroupsManager) Get(subsystem, name string) (string, error) {
	cgroupPath, err := m.SubsystemPath(subsystem)
	if err != nil {
		return "", fmt.Errorf("cgroups_manager: get: %s", err)
	}
	body, err := ioutil.ReadFile(path.Join(cgroupPath, name))
	if err != nil {
		return "", err
	}

	return strings.Trim(string(body), "\n"), nil
}

func (m *ContainerCgroupsManager) SubsystemPath(subsystem string) (string, error) {
	cgroupNode, err := m.cgroupReader.CgroupNode(subsystem)
	if err != nil {
		return "", err
	}
	return path.Join(m.cgroupsPath, subsystem, cgroupNode, "instance-"+m.containerID), nil
}
