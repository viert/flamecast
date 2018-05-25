package cast

import (
	"configreader"
	logging "github.com/op/go-logging"
	stdlog "log"
	"net/http"
	"os"
)

const (
	// FlamecastVersion holds the version of the flamecast server. Used in stats
	FlamecastVersion = "0.1.0"

	LoggerModule = "flamecast.server"
	LogFormat    = "%{color}%{level:6s}%{color:reset} %{message} [%{shortfunc}]"
)

var (
	logger         *logging.Logger
	config         *configreader.Config
	sourcesPathMap = make(map[string]*Source)
	stats          = new(StatsData)
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

	for path, sourceConfig := range config.SourcesPathMap {
		sourcesPathMap[path] = NewSource(sourceConfig)
	}

	stats.SourcesCount = len(sourcesPathMap)
	stats.ServerID = "Flamecast " + FlamecastVersion
	stats.Host, err = os.Hostname()
	stats.Admin = config.Admin
	if err != nil {
		stats.Host = "localhost"
	}

	return nil
}

// Start starts flamecast server
func Start() error {
	// Flamecast API
	http.HandleFunc("/api/v1/stats", statsHandler)

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
