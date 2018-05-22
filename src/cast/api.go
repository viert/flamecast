package cast

import (
	"configreader"
	"encoding/json"
	"icy"
	"net/http"
	"time"
)

type ListenerDesc struct {
	Key    string    `json:"key"`
	Joined time.Time `json:"joined_at"`
}

type SourceDesc struct {
	Active      bool           `json:"active"`
	Path        string         `json:"path"`
	Name        string         `json:"name"`
	Public      bool           `json:"public"`
	Site        string         `json:"site"`
	Genre       string         `json:"genre"`
	Description string         `json:"description"`
	Bitrate     int            `json:"bitrate"`
	Type        string         `json:"type"`
	PullURL     string         `json:"pull_url"`
	CurrentMeta icy.MetaData   `json:"current_meta"`
	Listeners   []ListenerDesc `json:"listeners"`
}

type SourcesListData struct {
	Data []SourceDesc `json:"data"`
}

func sourcesListHandler(rw http.ResponseWriter, req *http.Request) {
	sourcesListData := SourcesListData{make([]SourceDesc, 0, len(sourcesPathMap))}
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
			Listeners:   make([]ListenerDesc, 0, 512),
			CurrentMeta: source.currentMeta,
		}
		if source.config.Type == configreader.SourceTypePull {
			sd.Type = "pull"
			sd.PullURL = source.config.SourcePullUrl.String()
		} else if source.config.Type == configreader.SourceTypePush {
			sd.Type = "push"
		}

		source.listeners.Iter(func(lr *Listener) {
			ld := ListenerDesc{
				Key:    lr.key,
				Joined: lr.joined,
			}
			sd.Listeners = append(sd.Listeners, ld)
		})

		sourcesListData.Data = append(sourcesListData.Data, sd)
	}
	response, err := json.MarshalIndent(sourcesListData, "", "  ")
	if err != nil {
		http.Error(rw, err.Error(), 500)
		return
	}
	rw.Header().Set("Content-Type", "application/json")
	rw.Write(response)
}
