package cgroups_manager

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
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

func (m *ContainerCgroupsManager) Add(pid int, subsystems ...string) error {
	for _, subsystem := range subsystems {
		subsystemPath, err := m.SubsystemPath(subsystem)
		if err != nil {
			return err
		}

		tasks, err := os.OpenFile(path.Join(subsystemPath, "tasks"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			return err
		}
		defer tasks.Close()

		if _, err := fmt.Fprintf(tasks, "%d\n", pid); err != nil {
			return err
		}
	}
	return nil
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

func (m *ContainerCgroupsManager) Setup(subsystems ...string) error {
	for _, subsystem := range subsystems {
		subsystemPath, err := m.SubsystemPath(subsystem)
		if err != nil {
			return err
		}

		if err := os.MkdirAll(subsystemPath, 0755); err != nil {
			return err
		}

		if subsystem == "cpuset" {
			for _, file := range []string{"cpuset.cpus", "cpuset.mems"} {
				systemCgroup, err := os.Open(path.Join(m.cgroupsPath, subsystem, file))
				if err != nil {
					return fmt.Errorf("cgroups: initialize cpuset: %s", err)
				}

				defer systemCgroup.Close()
				subsystemCgroup, err := os.OpenFile(path.Join(subsystemPath, file), os.O_RDWR|os.O_CREATE, 0600)
				if err != nil {
					return fmt.Errorf("cgroups: initialize cpuset: %s", err)
				}

				defer subsystemCgroup.Close()
				if _, err := io.Copy(subsystemCgroup, systemCgroup); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
