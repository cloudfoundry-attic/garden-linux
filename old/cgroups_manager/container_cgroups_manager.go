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
	cgroupsPath string
	containerID string
}

func New(cgroupsPath, containerID string) *ContainerCgroupsManager {
	return &ContainerCgroupsManager{cgroupsPath, containerID}
}

func (m *ContainerCgroupsManager) Set(subsystem, name, value string) error {
	return ioutil.WriteFile(path.Join(m.SubsystemPath(subsystem), name), []byte(value), 0644)
}

func (m *ContainerCgroupsManager) Get(subsystem, name string) (string, error) {
	body, err := ioutil.ReadFile(path.Join(m.SubsystemPath(subsystem), name))
	if err != nil {
		return "", err
	}

	return strings.Trim(string(body), "\n"), nil
}

func (m *ContainerCgroupsManager) Setup(subsystems ...string) error {
	for _, subsystem := range subsystems {
		if err := os.MkdirAll(m.SubsystemPath(subsystem), 0755); err != nil {
			return err
		}

		if subsystem == "cpuset" {
			for _, file := range []string{"cpuset.cpus", "cpuset.mems"} {
				systemCgroup, err := os.Open(path.Join(m.cgroupsPath, subsystem, file))
				if err != nil {
					return fmt.Errorf("cgroups: initialize cpuset: %s", err)
				}

				defer systemCgroup.Close()
				subsystemCgroup, err := os.OpenFile(path.Join(m.SubsystemPath(subsystem), file), os.O_RDWR|os.O_CREATE, 0600)
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

func (m *ContainerCgroupsManager) Add(pid int, subsystems ...string) error {
	for _, subsystem := range subsystems {
		tasks, err := os.OpenFile(path.Join(m.SubsystemPath(subsystem), "tasks"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
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

func (m *ContainerCgroupsManager) SubsystemPath(subsystem string) string {
	return path.Join(m.cgroupsPath, subsystem, "instance-"+m.containerID)
}
