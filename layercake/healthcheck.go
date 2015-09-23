package layercake

import (
	"fmt"
	"io/ioutil"
	"os"
)

type GraphPath string

func (gp GraphPath) HealthCheck() error {
	f, err := ioutil.TempFile(string(gp), "healthprobe")
	if err != nil {
		return fmt.Errorf("graph directory '%s' is not writeable: %s", gp, err)
	}

	f.Close()
	os.Remove(f.Name())

	return nil
}
