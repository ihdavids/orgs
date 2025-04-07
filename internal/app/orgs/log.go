package orgs

/* SDOC: Settings
* Logging
  
  TODO: Fill in information on orgs server logging
EDOC */

import (
	"os"
	"strconv"

	logging "gopkg.in/op/go-logging.v1"
)

func IncreaseLogLevel(verbosity int) {
	logging.SetLevel(logging.GetLevel("")+logging.Level(verbosity), "")
}

func InitLogging() *logging.LeveledBackend {
	logBackend := logging.NewLogBackend(os.Stderr, "", 0)
	format := os.Getenv("ORGS_LOG_FORMAT")
	if format == "" {
		format = "%{color}%{level:-5s}%{color:reset} %{message}"
	}
	backend := logging.SetBackend(
		logging.NewBackendFormatter(
			logBackend,
			logging.MustStringFormatter(format),
		),
	)
	if os.Getenv("ORGS_DEBUG") == "" {
		logging.SetLevel(logging.NOTICE, "")
	} else {
		logging.SetLevel(logging.DEBUG, "")
		if verbosity, err := strconv.Atoi(os.Getenv("ORGS_DEBUG")); err == nil {
			IncreaseLogLevel(verbosity)
		}
	}
	return &backend
}

var orgsLogger *logging.Logger

func GetLog() *logging.Logger {
	if orgsLogger == nil {
		backend := InitLogging()
		orgsLogger = logging.MustGetLogger("orgs")
		orgsLogger.SetBackend(*backend)
	}
	return orgsLogger
}
