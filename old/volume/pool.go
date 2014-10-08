package volume

import (
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/cloudfoundry-incubator/garden-linux/old/linux_backend"
	"github.com/cloudfoundry-incubator/garden/api"
)

type Pool struct {
	globalVolumesPath string

	currentVolumeNum int64
}

func NewPool(globalVolumesPath string) *Pool {
	return &Pool{
		globalVolumesPath: globalVolumesPath,

		currentVolumeNum: time.Now().UnixNano(),
	}
}

func (pool *Pool) Setup() error {
	return os.MkdirAll(pool.globalVolumesPath, 0755)
}

func (pool *Pool) Create(spec api.VolumeSpec) (linux_backend.Volume, error) {
	id := pool.generateVolumeID()

	if spec.Handle == "" {
		spec.Handle = id
	}

	volumePath := filepath.Join(pool.globalVolumesPath, id)

	err := os.Mkdir(volumePath, 0755)
	if err != nil {
		return nil, err
	}

	if spec.HostPath != "" {
		err := syscall.Mount(
			spec.HostPath,
			volumePath,
			"",
			syscall.MS_BIND,
			"",
		)
		if err != nil {
			return nil, err
		}
	}

	return &volume{
		id:     id,
		handle: spec.Handle,
	}, nil
}

func (pool *Pool) Destroy(volume linux_backend.Volume) error {
	volumePath := filepath.Join(pool.globalVolumesPath, volume.ID())

	err := syscall.Unmount(volumePath, 0)
	if err != nil {
		if err != syscall.EINVAL {
			// errored for reason other than volume path not being a mount point
			return err
		}
	}

	// clean up (possibly empty in the case of a bind-mount) volume directory
	for {
		err := os.RemoveAll(volumePath)
		if err == nil {
			return nil
		}

		pathErr, ok := err.(*os.PathError)
		if !ok {
			return err
		}

		if pathErr.Err != syscall.EBUSY {
			return err
		}

		// error was EBUSY; retry as it seems to take the kernel a bit to reap
		// volume mountpoints. this is the same logic exercised in destroy.sh.

		time.Sleep(time.Second)
	}

	return nil
}

func (pool *Pool) generateVolumeID() string {
	containerNum := atomic.AddInt64(&pool.currentVolumeNum, 1)

	containerID := []byte{}

	var i uint
	for i = 0; i < 11; i++ {
		containerID = strconv.AppendInt(
			containerID,
			(containerNum>>(55-(i+1)*5))&31,
			32,
		)
	}

	return string(containerID)
}
