package configreader

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"

	logging "github.com/op/go-logging"
	"github.com/viert/properties"
)

// Default configuration values
const (
	DefaultBind              = ":8000"
	DefaultBitrate           = 96
	DefaultSourceUser        = "source"
	DefaultSourceType        = "PUSH"
	DefaultBroadcastAuthType = "NONE"
	DefaultLogfile           = "/var/log/flamecast.log"
	DefaultLogLevel          = "ERROR"
)

// SourceType valid values
const (
	SourceTypePush = iota
	SourceTypePull
)

// BroadcastAuthType valid values
const (
	BroadcastAuthTypeNone = iota
	BroadcastAuthTypeToken
)

// Defaults and mappings
var (
	DefaultSourceBitrates = [...]byte{96, 112}
	SourceTypes           = map[string]int{"PUSH": SourceTypePush, "PULL": SourceTypePull}
	AuthTypes             = map[string]int{"NONE": BroadcastAuthTypeNone, "TOKEN": BroadcastAuthTypeToken}
)

type (
	StreamDescription struct {
		Name        string
		Public      bool
		URL         string
		Genre       string
		Description string
		Bitrate     int
		AudioInfo   string
	}

	SourceConfig struct {
		Name                       string
		Path                       string
		FallbackPath               string
		Type                       int
		SourceAuthToken            string
		SourcePullURL              *url.URL
		Stream                     StreamDescription
		BroadcastAuthType          int
		BroadcastAuthTokenCheckURL *url.URL
		BroadcastNotifyEnterURL    *url.URL
		BroadcastNotifyLeaveURL    *url.URL
	}

	Config struct {
		Admin          string
		Bind           string
		LogFile        string
		LogLevel       logging.Level
		SourcesNameMap map[string]*SourceConfig
		SourcesPathMap map[string]*SourceConfig
	}
)

func sourceTypeFromString(srcType string) int {
	return SourceTypes[srcType]
}

func validSourceTypes() string {
	st := make([]string, 0, len(SourceTypes))
	for k := range SourceTypes {
		st = append(st, fmt.Sprintf("\"%s\"", strings.ToLower(k)))
	}
	return strings.Join(st, ", ")
}

func isValidSourceType(srcType string) bool {
	_, exists := SourceTypes[srcType]
	return exists
}

func broadcastAuthTypeFromString(authType string) int {
	return AuthTypes[authType]
}

func validAuthTypes() string {
	at := make([]string, 0, len(AuthTypes))
	for k := range AuthTypes {
		at = append(at, fmt.Sprintf("\"%s\"", strings.ToLower(k)))
	}
	return strings.Join(at, ", ")
}

func isValidAuthType(authType string) bool {
	_, exists := AuthTypes[authType]
	return exists
}

// Load loads and parses config with a given filename
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
		cfg.Bind = DefaultBind
	}

	cfg.LogFile, err = props.GetString("main.log.file")
	if err != nil {
		cfg.LogFile = DefaultLogfile
	}

	logLevel, err := props.GetString("main.log.level")
	if err != nil {
		logLevel = DefaultLogLevel
	}
	cfg.LogLevel, err = logging.LogLevel(strings.ToUpper(logLevel))
	if err != nil {
		return nil, errors.New("Invalid log level: " + logLevel)
	}

	cfg.Admin, _ = props.GetString("main.admin")

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

		// SourceAuthToken
		var user, password string
		user, err = props.GetString(prefix + "source.auth.user")
		if err != nil {
			user = DefaultSourceUser
		}
		password, err = props.GetString(prefix + "source.auth.password")
		if err != nil {
			password = "?"
		}
		scfg.SourceAuthToken = base64.StdEncoding.EncodeToString([]byte(user + ":" + password))

		if scfg.Type == SourceTypePull {
			srcURL, err := props.GetString(prefix + "source.url")
			if err != nil {
				return nil, errors.New("No source.url for PULL-type source " + sourceName)
			}
			scfg.SourcePullURL, err = url.Parse(srcURL)
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
			authURL, err := props.GetString(prefix + "broadcast.auth.token_check_url")
			if err != nil {
				return nil, errors.New("No broadcast.auth.token_check_url (while broadcast.auth.type is TOKEN) for source " + sourceName)
			}
			scfg.BroadcastAuthTokenCheckURL, err = url.Parse(authURL)
			if err != nil {
				return nil, errors.New("Invalid broadcast.auth.token_check_url for source " + sourceName + ": " + err.Error())
			}
		}

		notifyEnter, err := props.GetString(prefix + "broadcast.notify.enter")
		if err == nil {
			scfg.BroadcastNotifyEnterURL, err = url.Parse(notifyEnter)
			if err != nil {
				return nil, errors.New("Invalid URL in broadcast.notify.enter for source " + sourceName + ": " + err.Error())

			}
		}

		notifyLeave, err := props.GetString(prefix + "broadcast.notify.leave")
		if err == nil {
			scfg.BroadcastNotifyLeaveURL, err = url.Parse(notifyLeave)
			if err != nil {
				return nil, errors.New("Invalid URL in broadcast.notify.leave for source " + sourceName + ": " + err.Error())

			}
		}

		scfg.Stream.Name, _ = props.GetString(prefix + "source.name")
		scfg.Stream.Description, _ = props.GetString(prefix + "source.description")
		scfg.Stream.Bitrate, err = props.GetInt(prefix + "source.bitrate")
		if err != nil {
			scfg.Stream.Bitrate = DefaultBitrate
		}
		scfg.Stream.AudioInfo = fmt.Sprintf("br=%d", scfg.Stream.Bitrate)
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
