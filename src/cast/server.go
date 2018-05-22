package cast

import (
	"configreader"
	logging "github.com/op/go-logging"
	stdlog "log"
	"net/http"
	"os"
)

const (
	LoggerModule = "flamecast.server"
	LogFormat    = "%{color}%{level:.4s} %{id:03x}%{color:reset} %{shortfunc} %{message}"
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
	logger = logging.MustGetLogger(LoggerModule)
	logging.SetLevel(config.LogLevel, LoggerModule)
	logging.SetFormatter(logging.MustStringFormatter(LogFormat))
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
	// Flamecast API
	http.HandleFunc("/api/v1/sources", sourcesListHandler)

	// Icecast compatibility API
	http.HandleFunc("/admin/metadata", adminMetadataHandler)

	// Main handler for feeding and listening to sources
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
		logger.Errorf("error starting server: %s", err.Error())
	}
	return err
}
