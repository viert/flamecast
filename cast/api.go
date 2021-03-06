package cast

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/viert/flamecast/configreader"
	"github.com/viert/flamecast/icy"
)

type (
	// ListenerDesc describes json representation of a listener
	ListenerDesc struct {
		Key        string    `json:"key"`
		Joined     time.Time `json:"joined_at"`
		RemoteAddr string    `json:"remote_addr"`
	}

	// SourceDesc describes json representation of a source
	SourceDesc struct {
		Active      bool           `json:"active"`
		Path        string         `json:"path"`
		Name        string         `json:"name"`
		Public      bool           `json:"public"`
		Site        string         `json:"site"`
		Genre       string         `json:"genre"`
		Description string         `json:"description"`
		Bitrate     int            `json:"bitrate"`
		AudioInfo   string         `json:"audio_info"`
		Type        string         `json:"type"`
		Started     string         `json:"started"`
		ContentType string         `json:"content_type"`
		CurrentMeta icy.MetaData   `json:"current_meta"`
		Listeners   []ListenerDesc `json:"listeners"`
	}

	// StatsData contains server stats close to what icecast stats handler provides
	StatsData struct {
		Admin               string       `json:"admin"`
		Host                string       `json:"host"`
		ListenerConnections uint64       `json:"listener_connections"`
		FeederConnections   uint64       `json:"feeder_connections"`
		PullerConnections   uint64       `json:"puller_connections"`
		ListenersCount      uint         `json:"listeners_count"`
		ServerID            string       `json:"server_id"`
		SourcesCount        int          `json:"sources_count"`
		Sources             []SourceDesc `json:"sources"`
	}
)

func adminMetadataHandler(rw http.ResponseWriter, req *http.Request) {
	values := req.URL.Query()
	mount := values.Get("mount")
	if mount == "" {
		http.Error(rw, "mount param is missing", http.StatusBadRequest)
		return
	}
	source, found := sourcesPathMap[mount]
	if !found {
		http.Error(rw, "mount not found", http.StatusNotFound)
		return
	}

	if !checkSourceAuth(source, req) {
		http.Error(rw, "authorization failed", http.StatusUnauthorized)
		return
	}

	mode := values.Get("mode")
	if mode == "" {
		http.Error(rw, "mode param is missing", http.StatusBadRequest)
		return
	}
	if mode != "updinfo" {
		http.Error(rw, "mode param is invalid", http.StatusBadRequest)
		return
	}
	song := values.Get("song")
	if song == "" {
		http.Error(rw, "song param is missing", http.StatusBadRequest)
		return
	}

	meta := icy.MetaData{"StreamTitle": song}
	setSourceMetadata(source, meta)
	rw.Write([]byte("metadata changed"))
}

func statsHandler(rw http.ResponseWriter, req *http.Request) {
	sourcesListData := make([]SourceDesc, 0, len(sourcesPathMap))
	for path, source := range sourcesPathMap {
		sd := SourceDesc{
			Active:      source.active,
			Path:        path,
			Name:        source.config.Stream.Name,
			Public:      source.config.Stream.Public,
			Site:        source.config.Stream.URL,
			Genre:       source.config.Stream.Genre,
			Description: source.config.Stream.Description,
			Bitrate:     source.config.Stream.Bitrate,
			AudioInfo:   source.config.Stream.AudioInfo,
			Listeners:   make([]ListenerDesc, 0, 512),
			CurrentMeta: source.currentMeta,
			ContentType: source.ContentType,
		}
		if sd.Active {
			sd.Started = source.Started.Format(time.RFC3339)
		} else {
			sd.Started = ""
		}
		if source.config.Type == configreader.SourceTypePull {
			sd.Type = "pull"
		} else if source.config.Type == configreader.SourceTypePush {
			sd.Type = "push"
		}

		source.listeners.iter(func(lr *Listener) {
			ld := ListenerDesc{
				Key:        lr.key,
				Joined:     lr.joined,
				RemoteAddr: lr.request.RemoteAddr,
			}
			sd.Listeners = append(sd.Listeners, ld)
		})

		sourcesListData = append(sourcesListData, sd)
	}
	stats.Sources = sourcesListData
	response, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	rw.Header().Set("Content-Type", "application/json")
	rw.Write(response)
}
