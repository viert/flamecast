package cast

import (
	"configreader"
	logging "github.com/op/go-logging"
	stdlog "log"
	"net/http"
	"os"
)

const (
	LOGGER_MODULE    = "flamecast.server"
	PULL_RETRIES_MAX = 30
	PULL_BUFFER_SIZE = 8192
)

var (
	logger         *logging.Logger
	config         *configreader.Config
	sourcesPathMap map[string]*Source
)

func Configure(cfg *configreader.Config) error {
	config = cfg

	// Setting up logger
	lf, err := os.OpenFile(config.LogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	logger = logging.MustGetLogger(LOGGER_MODULE)
	logging.SetLevel(config.LogLevel, LOGGER_MODULE)
	logging.SetFormatter(logging.MustStringFormatter("%{level} %{message}"))
	fileBackend := logging.NewLogBackend(lf, "", stdlog.LstdFlags)
	stderrBackend := logging.NewLogBackend(os.Stderr, "", stdlog.LstdFlags)
	stderrBackend.Color = true
	logging.SetBackend(fileBackend, stderrBackend)

	sourcesPathMap = make(map[string]*Source)

	for path, sourceConfig := range config.SourcesPathMap {
		sourcesPathMap[path] = NewSource(sourceConfig)
	}

	return nil
}

func Start() error {
	http.HandleFunc("/api/v1/sources", sourcesListHandler)
	http.HandleFunc("/", sourceHandler)

	for path, source := range sourcesPathMap {
		if source.config.Type == configreader.SourceTypePull {
			logger.Noticef("Starting pulling thread for source %s", path)
			go pullSource(source)
		}
	}

	logger.Notice("Server is starting")
	err := http.ListenAndServe(config.Bind, nil)
	if err != nil {
		logger.Error("%s", err)
	}
	return err
}
