package debug

import (
	"bytes"
	"expvar"
	"fmt"
	"io/ioutil"
	"net/http"
	"os/exec"
	"runtime"
	"strings"

	"github.com/cloudfoundry-incubator/cf-debug-server"
	"github.com/pivotal-golang/lager"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/http_server"
)

func Run(address string, sink *lager.ReconfigurableSink, backingStoresPath, depotDirs string) (ifrit.Process, error) {
	expvar.Publish("numCPUS", expvar.Func(func() interface{} {
		return int64(runtime.NumCPU())
	}))

	expvar.Publish("numGoRoutines", expvar.Func(func() interface{} {
		return int64(runtime.NumGoroutine())
	}))

	expvar.Publish("loopDevices", expvar.Func(func() interface{} {
		devices, err := exec.Command("losetup", "-a").CombinedOutput()
		if err != nil {
			return fmt.Errorf("%s, out: %s", err, string(devices))
		}
		return bytes.Count(devices, []byte("\n"))
	}))

	expvar.Publish("backingStores", expvar.Func(func() interface{} {
		entries, err := ioutil.ReadDir(backingStoresPath)
		if err != nil {
			return err
		}

		return len(entries)
	}))

	expvar.Publish("depotDirs", expvar.Func(func() interface{} {
		entries, err := ioutil.ReadDir(depotDirs)
		if err != nil {
			return err
		}

		return len(entries)
	}))

	server := http_server.New(address, handler(sink))
	p := ifrit.Invoke(server)
	select {
	case <-p.Ready():
	case err := <-p.Wait():
		return nil, err
	}
	return p, nil
}

func handler(sink *lager.ReconfigurableSink) http.Handler {
	pprofHandler := cf_debug_server.Handler(sink)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/debug/vars") {
			http.DefaultServeMux.ServeHTTP(w, r)
			return
		}
		pprofHandler.ServeHTTP(w, r)
	})
}
