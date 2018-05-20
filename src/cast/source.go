package cast

import (
	"configreader"
	"mpeg"
	"net/http"
	"strconv"
)

const (
	InitialListenersCount = 256
)

type (
	Source struct {
		config       *configreader.SourceConfig
		inputChannel chan []byte
		listeners    *ListenerSlice
		active       bool
	}

	SourceStat struct {
		BytesRead   uint64
		FramesFound uint64
	}
)

func NewSource(config *configreader.SourceConfig) *Source {
	return &Source{
		config,
		make(chan []byte, 1024),
		NewListenerSlice(512),
		false,
	}
}

func pullSource(source *Source) {
	retriesLeft := PULL_RETRIES_MAX

	sourceUrl := source.config.SourcePullUrl.String()
	sourcePath := source.config.Path

	cli := new(http.Client)

retryLoop:
	for retriesLeft > 0 {
		req, err := http.NewRequest("GET", sourceUrl, nil)
		if err != nil {
			logger.Errorf("SOURCE \"%s\": error creating request: %s", sourcePath, err.Error())
			retriesLeft--
			continue retryLoop
		}

		req.Header.Set("Icy-MetaData", "1")
		resp, err := cli.Do(req)
		if err != nil {
			logger.Errorf("SOURCE \"%s\": error pulling source: %s", sourcePath, err.Error())
			retriesLeft--
			continue retryLoop
		}

		var metaInterval int64 = 0
		miString := resp.Header.Get("icy-metaint")
		if miString != "" {
			metaInterval, _ = strconv.ParseInt(miString, 10, 64)
		}

		logger.Noticef("SOURCE \"%s\": source puller connected", sourcePath)
		source.active = true

		parser := mpeg.NewParser(resp.Body, int(metaInterval))
		for {
			frame, _, frameType := parser.Iter()
			switch frameType {
			case mpeg.FrameTypeNone:
				logger.Errorf("SOURCE \"%s\": no data from source, retrying", sourcePath)
				continue retryLoop
			case mpeg.FrameTypeMeta:
				logger.Noticef("SOURCE \"%s\": got metadata of len %d", sourcePath, len(frame))
			default:
				source.inputChannel <- frame
			}
		}
	}
	source.active = false
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
