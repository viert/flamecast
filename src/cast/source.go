package cast

import (
	"configreader"
	"github.com/tcolgate/mp3"
	"net/http"
)

const (
	InitialListenersCount = 256
)

type Source struct {
	config       *configreader.SourceConfig
	inputChannel chan *mp3.Frame
	listeners    *ListenerSlice
	active       bool
}

func NewSource(config *configreader.SourceConfig) *Source {
	return &Source{
		config,
		make(chan *mp3.Frame, 5120),
		NewListenerSlice(512),
		false,
	}
}

func pullSource(source *Source) {
	retriesLeft := PULL_RETRIES_MAX

	sourceUrl := source.config.SourcePullUrl.String()
	sourcePath := source.config.Path
	source.active = true

retryLoop:
	for retriesLeft > 0 {
		resp, err := http.Get(sourceUrl)
		if err != nil {
			logger.Errorf("Error pulling source %s: %s", sourcePath, err)
			retriesLeft--
			continue retryLoop
		}

		logger.Noticef("Source puller for source %s connected", sourcePath)
		var frame *mp3.Frame
		decoder := mp3.NewDecoder(resp.Body)
		for {
			frame = new(mp3.Frame)
			var skipped int
			err = decoder.Decode(frame, &skipped)
			if err != nil {
				logger.Errorf("Error decoding data: %s", err)
				continue retryLoop
			}
			source.inputChannel <- frame
		}
	}

}

func sourceHandler(rw http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case "SOURCE":
		logger.Error("SOURCE method is not implemented")
		http.Error(rw, "SOURCE method is not implemented", http.StatusMethodNotAllowed)

	case "GET":
		handleListener(rw, req)
	}
}

func (s *Source) multiplex() {
	for frame := range s.inputChannel {
		s.listeners.Iter(func(lr *Listener) {
			select {
			case lr.frameBuffer <- frame:
			default:
			}
		})
	}
}
