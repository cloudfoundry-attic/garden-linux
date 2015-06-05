package cf_debug_server

import (
	"flag"
	"io/ioutil"
	"net/http"
	"net/http/pprof"
	"strconv"

	"github.com/pivotal-golang/lager"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/http_server"
)

const (
	DebugFlag = "debugAddr"
)

func AddFlags(flags *flag.FlagSet) {
	flags.String(
		DebugFlag,
		"",
		"host:port for serving pprof debugging info",
	)
}

func DebugAddress(flags *flag.FlagSet) string {
	dbgFlag := flags.Lookup(DebugFlag)
	if dbgFlag == nil {
		return ""
	}

	return dbgFlag.Value.String()
}

func Runner(address string, sink *lager.ReconfigurableSink) ifrit.Runner {
	return http_server.New(address, Handler(sink))
}

func Run(address string, sink *lager.ReconfigurableSink) (ifrit.Process, error) {
	p := ifrit.Invoke(Runner(address, sink))
	select {
	case <-p.Ready():
	case err := <-p.Wait():
		return nil, err
	}
	return p, nil
}

func Handler(sink *lager.ReconfigurableSink) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/debug/pprof/", http.HandlerFunc(pprof.Index))
	mux.Handle("/debug/pprof/cmdline", http.HandlerFunc(pprof.Cmdline))
	mux.Handle("/debug/pprof/profile", http.HandlerFunc(pprof.Profile))
	mux.Handle("/debug/pprof/symbol", http.HandlerFunc(pprof.Symbol))
	mux.Handle("/log-level", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		level, err := ioutil.ReadAll(r.Body)
		if err != nil {
			return
		}

		switch string(level) {
		case "debug", "DEBUG", "d", strconv.Itoa(int(lager.DEBUG)):
			sink.SetMinLevel(lager.DEBUG)
		case "info", "INFO", "i", strconv.Itoa(int(lager.INFO)):
			sink.SetMinLevel(lager.INFO)
		case "error", "ERROR", "e", strconv.Itoa(int(lager.ERROR)):
			sink.SetMinLevel(lager.ERROR)
		case "fatal", "FATAL", "f", strconv.Itoa(int(lager.FATAL)):
			sink.SetMinLevel(lager.FATAL)
		}
	}))

	return mux
}
