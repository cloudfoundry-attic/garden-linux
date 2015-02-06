package container_pool

import (
	"encoding/json"
	"fmt"
	"os"
	"path"

	"github.com/cloudfoundry-incubator/garden-linux/network/cnet"
	"github.com/pivotal-golang/lager"
)

type RawCN struct {
	CNRawMessage *json.RawMessage
}

type CNPersistor interface {
	Persist(cn cnet.ContainerNetwork, path string) error
	Recover(path string) (cnet.ContainerNetwork, error)
}

type cnPersistor struct {
	logger    lager.Logger
	cnBuilder cnet.Builder
}

func NewCNPersistor(logger lager.Logger, cnBuilder cnet.Builder) CNPersistor {
	return &cnPersistor{
		logger:    logger,
		cnBuilder: cnBuilder,
	}
}

func (f *cnPersistor) Persist(cn cnet.ContainerNetwork, path string) error {
	err := os.MkdirAll(path, 0755)
	if err != nil {
		f.logger.Error("persist-directory-not-created", err, lager.Data{"path": path})
		return fmt.Errorf("Cannot create persistor directory %q: %s", path, err)
	}

	var m json.RawMessage
	m, err = cn.MarshalJSON()
	if err != nil {
		f.logger.Error("persist-marshall-cnet-error", err, lager.Data{"path": path, "cnet": cn.String()})
		return fmt.Errorf("Cannot marshall cnet %#v: %s", cn, err)
	}

	cnConfigPath := cnConfigPath(path)

	out, err := os.Create(cnConfigPath)
	if err != nil {
		f.logger.Error("persist-file-create-error", err, lager.Data{"path": path})
		return fmt.Errorf("Cannot create persistor file %q: %s", path, err)
	}
	defer out.Close()

	rf := RawCN{&m}
	err = json.NewEncoder(out).Encode(rf)
	if err != nil {
		f.logger.Error("persist-encode-error", err, lager.Data{"path": path, "cnet": cn.String()})
		return fmt.Errorf("Cannot encode cnet %#v to path %q: %s", cn, path, err)
	}

	return nil
}

func (f *cnPersistor) Recover(path string) (cnet.ContainerNetwork, error) {
	cnConfigPath := cnConfigPath(path)

	in, err := os.Open(cnConfigPath)
	if err != nil {
		f.logger.Error("recover-persist-file-error", err, lager.Data{"cnetConfigPath": cnConfigPath})
		return nil, fmt.Errorf("Cannot open persistor file %q: %s", cnConfigPath, err)
	}
	defer in.Close()

	var rf RawCN
	err = json.NewDecoder(in).Decode(&rf)
	if err != nil {
		f.logger.Error("recover-persist-file-decode-error", err, lager.Data{"cnetConfigPath": cnConfigPath})
		return nil, fmt.Errorf("Cannot decode persistor file %q: %s", cnConfigPath, err)
	}

	cn, err := f.cnBuilder.Rebuild(rf.CNRawMessage)
	if err != nil {
		f.logger.Error("recover-rebuild-error", err, lager.Data{"cnetConfigPath": cnConfigPath})
		return nil, fmt.Errorf("Cannot rebuild cnet %q: %s", cnConfigPath, err)
	}

	return cn, nil
}

func cnConfigPath(containerPath string) string {
	return path.Join(containerPath, "cnetConfig.json")
}
