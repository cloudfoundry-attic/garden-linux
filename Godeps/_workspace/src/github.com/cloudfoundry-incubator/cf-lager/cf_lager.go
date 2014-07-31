package cf_lager

import (
	"flag"
	"fmt"
	"os"

	"github.com/pivotal-golang/lager"
)

const (
	DEBUG = "debug"
	INFO  = "info"
	ERROR = "error"
	FATAL = "fatal"
)

var enableSyslog bool
var syslogPrefix string
var minLogLevel string

func init() {
	flag.StringVar(
		&minLogLevel,
		"logLevel",
		string(INFO),
		"log level: debug, info, error or fatal",
	)
}

func New(component string) lager.Logger {
	if !flag.Parsed() {
		flag.Parse()
	}
	var minLagerLogLevel lager.LogLevel
	switch minLogLevel {
	case DEBUG:
		minLagerLogLevel = lager.DEBUG
	case INFO:
		minLagerLogLevel = lager.INFO
	case ERROR:
		minLagerLogLevel = lager.ERROR
	case FATAL:
		minLagerLogLevel = lager.FATAL
	default:
		panic(fmt.Errorf("unkown log level: %s", minLogLevel))
	}

	logger := lager.NewLogger(component)
	logger.RegisterSink(lager.NewWriterSink(os.Stdout, minLagerLogLevel))

	return logger
}
