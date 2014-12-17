package container_pool

import (
	"encoding/json"
	"fmt"
	"os"
	"path"

	"github.com/cloudfoundry-incubator/garden-linux/fences"
	"github.com/pivotal-golang/lager"
)

type RawFence struct {
	FenceRawMessage *json.RawMessage
}

type FencePersistor interface {
	Persist(fence fences.Fence, path string) error
	Recover(path string) (fences.Fence, error)
}

type fencePersistor struct {
	logger       lager.Logger
	fenceBuilder FenceBuilder
}

func NewFencePersistor(logger lager.Logger, fenceBuilder FenceBuilder) FencePersistor {
	return &fencePersistor{
		logger:       logger,
		fenceBuilder: fenceBuilder,
	}
}

func (f *fencePersistor) Persist(fence fences.Fence, path string) error {
	err := os.MkdirAll(path, 0755)
	if err != nil {
		f.logger.Error("persist-directory-not-created", err, lager.Data{"path": path})
		return fmt.Errorf("Cannot create persistor directory %q: %s", path, err)
	}

	var m json.RawMessage
	m, err = fence.MarshalJSON()
	if err != nil {
		f.logger.Error("persist-marshall-fence-error", err, lager.Data{"path": path, "fence": fence.String()})
		return fmt.Errorf("Cannot marshall fence %#v: %s", fence, err)
	}

	fenceConfigPath := fenceConfigPath(path)

	out, err := os.Create(fenceConfigPath)
	if err != nil {
		f.logger.Error("persist-file-create-error", err, lager.Data{"path": path})
		return fmt.Errorf("Cannot create persistor file %q: %s", path, err)
	}
	defer out.Close()

	rf := RawFence{&m}
	err = json.NewEncoder(out).Encode(rf)
	if err != nil {
		f.logger.Error("persist-encode-error", err, lager.Data{"path": path, "fence": fence.String()})
		return fmt.Errorf("Cannot encode fence %#v to path %q: %s", fence, path, err)
	}

	return nil
}

func (f *fencePersistor) Recover(path string) (fences.Fence, error) {
	fenceConfigPath := fenceConfigPath(path)

	in, err := os.Open(fenceConfigPath)
	if err != nil {
		f.logger.Error("recover-persist-file-error", err, lager.Data{"fenceConfigPath": fenceConfigPath})
		return nil, fmt.Errorf("Cannot open persistor file %q: %s", fenceConfigPath, err)
	}
	defer in.Close()

	var rf RawFence
	err = json.NewDecoder(in).Decode(&rf)
	if err != nil {
		f.logger.Error("recover-persist-file-decode-error", err, lager.Data{"fenceConfigPath": fenceConfigPath})
		return nil, fmt.Errorf("Cannot decode persistor file %q: %s", fenceConfigPath, err)
	}

	fence, err := f.fenceBuilder.Rebuild(rf.FenceRawMessage)
	if err != nil {
		f.logger.Error("recover-rebuild-error", err, lager.Data{"fenceConfigPath": fenceConfigPath})
		return nil, fmt.Errorf("Cannot rebuild fence %q: %s", fenceConfigPath, err)
	}

	return fence, nil
}

func fenceConfigPath(containerPath string) string {
	return path.Join(containerPath, "fenceConfig.json")
}
