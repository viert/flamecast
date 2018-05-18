package configreader

import (
	"encoding/base64"
	"errors"
	"fmt"
	logging "github.com/op/go-logging"
	"github.com/viert/properties"
	"net/url"
	"strings"
)

const (
	DEFAULT_BIND                = ":8000"
	DEFAULT_SOURCE_USER         = "source"
	DEFAULT_SOURCE_TYPE         = "PUSH"
	DEFAULT_BROADCAST_AUTH_TYPE = "NONE"
	DEFAULT_LOG_FILE            = "/var/log/flamecast.log"
	DEFAULT_LOG_LEVEL           = "ERROR"
)

const (
	SourceTypePush = iota
	SourceTypePull
	BroadcastAuthTypeNone
	BroadcastAuthTypeToken
)

var (
	DEFAULT_SOURCE_BITRATES = [...]byte{96, 112}
	SOURCE_TYPES            = map[string]int{"PUSH": SourceTypePush, "PULL": SourceTypePull}
	AUTH_TYPES              = map[string]int{"NONE": BroadcastAuthTypeNone, "TOKEN": BroadcastAuthTypeToken}
)

type StreamDescription struct {
	Name        string `json:"name"`
	Public      bool   `json:"public"`
	URL         string `json:"url"`
	Genre       string `json:"genre"`
	Description string `json:"description"`
	Bitrate     int    `json:"bitrate"`
}

type SourceConfig struct {
	Name                       string
	Path                       string
	FallbackPath               string
	Type                       int
	SourceAuthToken            string
	SourcePullUrl              *url.URL
	Stream                     StreamDescription
	BroadcastAuthType          int
	BroadcastAuthTokenCheckUrl *url.URL
	BroadcastNotifyEnterUrl    *url.URL
	BroadcastNotifyLeaveUrl    *url.URL
}

type Config struct {
	Bind           string
	LogFile        string
	LogLevel       logging.Level
	SourcesNameMap map[string]*SourceConfig
	SourcesPathMap map[string]*SourceConfig
}

func sourceTypeFromString(srcType string) int {
	return SOURCE_TYPES[srcType]
}

func validSourceTypes() string {
	sourceTypes := make([]string, 0, len(SOURCE_TYPES))
	for k := range SOURCE_TYPES {
		sourceTypes = append(sourceTypes, fmt.Sprintf("\"%s\"", strings.ToLower(k)))
	}
	return strings.Join(sourceTypes, ", ")
}

func isValidSourceType(srcType string) bool {
	_, exists := SOURCE_TYPES[srcType]
	return exists
}

func broadcastAuthTypeFromString(authType string) int {
	return AUTH_TYPES[authType]
}

func validAuthTypes() string {
	authTypes := make([]string, 0, len(AUTH_TYPES))
	for k := range AUTH_TYPES {
		authTypes = append(authTypes, fmt.Sprintf("\"%s\"", strings.ToLower(k)))
	}
	return strings.Join(authTypes, ", ")
}

func isValidAuthType(authType string) bool {
	_, exists := AUTH_TYPES[authType]
	return exists
}

func Load(filename string) (*Config, error) {
	props, err := properties.Load(filename)

	if err != nil {
		return nil, err
	}

	cfg := &Config{
		SourcesNameMap: make(map[string]*SourceConfig),
		SourcesPathMap: make(map[string]*SourceConfig),
	}

	// Server-wide options configuration
	cfg.Bind, err = props.GetString("main.bind")
	if err != nil {
		cfg.Bind = DEFAULT_BIND
	}

	cfg.LogFile, err = props.GetString("main.log.file")
	if err != nil {
		cfg.LogFile = DEFAULT_LOG_FILE
	}

	logLevel, err := props.GetString("main.log.level")
	if err != nil {
		logLevel = DEFAULT_LOG_LEVEL
	}
	cfg.LogLevel, err = logging.LogLevel(strings.ToUpper(logLevel))
	if err != nil {
		return nil, errors.New("Invalid log level: " + logLevel)
	}

	if !props.KeyExists("sources") {
		return nil, errors.New("No [sources.*] sections found")
	}

	sourceNames, err := props.Subkeys("sources")
	if err != nil {
		return nil, err
	}

	// Sources configuration
	for _, sourceName := range sourceNames {
		var sourcePath, prefix string

		// name
		scfg := new(SourceConfig)
		scfg.Name = sourceName

		prefix = "sources." + sourceName + "."

		// path
		sourcePath, err = props.GetString(prefix + "source.path")
		if err != nil {
			sourcePath = "/" + sourceName
		}

		// TODO: check path for validity (/[a-z0-9_-\.]+)
		scfg.Path = sourcePath

		// Type
		sourceType, err := props.GetString(prefix + "source.type")
		if err != nil {
			return nil, errors.New("No source.type for source " + sourceName)
		}
		sourceType = strings.ToUpper(sourceType)
		if !isValidSourceType(sourceType) {
			return nil, errors.New("Invalid source type " + sourceType + " for source " + sourceName + ". Valid types are " + validSourceTypes())
		}
		scfg.Type = sourceTypeFromString(sourceType)

		switch scfg.Type {
		case SourceTypePush:
			// SourceAuthToken
			var user, password string
			user, err = props.GetString(prefix + "source.auth.user")
			if err != nil {
				user = DEFAULT_SOURCE_USER
			}
			password, err = props.GetString(prefix + "source.auth.password")
			if err != nil {
				return nil, errors.New("No source.auth.password for PUSH-type source " + sourceName)
			}
			scfg.SourceAuthToken = base64.StdEncoding.EncodeToString([]byte(user + ":" + password))
		case SourceTypePull:
			// SourcePullUrl
			srcUrl, err := props.GetString(prefix + "source.url")
			if err != nil {
				return nil, errors.New("No source.url for PULL-type source " + sourceName)
			}

			scfg.SourcePullUrl, err = url.Parse(srcUrl)
			if err != nil {
				return nil, errors.New("Invalid source.url for source " + sourceName + ": " + err.Error())
			}
		}

		broadcastAuthType, err := props.GetString(prefix + "broadcast.auth.type")
		if err != nil {
			broadcastAuthType = "NONE"
		}
		broadcastAuthType = strings.ToUpper(broadcastAuthType)
		if !isValidAuthType(broadcastAuthType) {
			return nil, errors.New("Invalid broadcast.auth.type for source " + sourceName + ", valid types are " + validAuthTypes())
		}

		scfg.BroadcastAuthType = broadcastAuthTypeFromString(broadcastAuthType)

		switch scfg.BroadcastAuthType {
		case BroadcastAuthTypeToken:
			authUrl, err := props.GetString(prefix + "broadcast.auth.token_check_url")
			if err != nil {
				return nil, errors.New("No broadcast.auth.token_check_url (while broadcast.auth.type is TOKEN) for source " + sourceName)
			}
			scfg.BroadcastAuthTokenCheckUrl, err = url.Parse(authUrl)
			if err != nil {
				return nil, errors.New("Invalid broadcast.auth.token_check_url for source " + sourceName + ": " + err.Error())
			}
		}

		notifyEnter, err := props.GetString(prefix + "broadcast.notify.enter")
		if err == nil {
			scfg.BroadcastNotifyEnterUrl, err = url.Parse(notifyEnter)
			if err != nil {
				return nil, errors.New("Invalid URL in broadcast.notify.enter for source " + sourceName + ": " + err.Error())

			}
		}

		notifyLeave, err := props.GetString(prefix + "broadcast.notify.leave")
		if err == nil {
			scfg.BroadcastNotifyLeaveUrl, err = url.Parse(notifyLeave)
			if err != nil {
				return nil, errors.New("Invalid URL in broadcast.notify.leave for source " + sourceName + ": " + err.Error())

			}
		}

		scfg.Stream.Name, _ = props.GetString(prefix + "source.name")
		scfg.Stream.Description, _ = props.GetString(prefix + "source.description")
		scfg.Stream.Bitrate, _ = props.GetInt(prefix + "source.bitrate")
		scfg.Stream.Public, _ = props.GetBool(prefix + "source.public")
		scfg.Stream.Genre, _ = props.GetString(prefix + "source.genre")
		scfg.Stream.URL, _ = props.GetString(prefix + "source.site")

		cfg.SourcesNameMap[sourceName] = scfg
		cfg.SourcesPathMap[sourcePath] = scfg
	}

	// Second pass configuration - fallback sources
	for sourceName, source := range cfg.SourcesNameMap {
		fallbackName, err := props.GetString("sources." + sourceName + ".source.fallback")
		if err != nil {
			continue
		}
		fallbackSource, ok := cfg.SourcesNameMap[fallbackName]
		if !ok {
			return nil, errors.New("Invalid fallback '" + fallbackName + "' for source " + sourceName)
		}
		source.FallbackPath = fallbackSource.Path
	}

	return cfg, nil
}
