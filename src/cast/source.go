package cast

import (
	"configreader"
	"github.com/viert/endless"
	"icy"
	"net/http"
	"strconv"
)

const (
	InitialListenersCount = 256
	DataBufferSize        = 4096
	EndlessSize           = 512 * 1024
)

type (
	Source struct {
		config      *configreader.SourceConfig
		Buffer      *endless.Endless
		metaChannel chan icy.MetaData
		currentMeta icy.MetaData
		listeners   *ListenerSlice
		active      bool
	}

	SourceStat struct {
		BytesRead   uint64
		FramesFound uint64
	}
)

func NewSource(config *configreader.SourceConfig) *Source {
	return &Source{
		config,
		endless.NewEndless(EndlessSize),
		make(chan icy.MetaData, 1),
		make(icy.MetaData),
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

		mfChannel := make(chan icy.MetaFrame, 1)
		reader := icy.NewReader(resp.Body, int(metaInterval), mfChannel)
		dataBuf := make([]byte, DataBufferSize)
		for {

			n, err := reader.Read(dataBuf)
			if err != nil {
				logger.Errorf("SOURCE \"%s\": error reading data: %s", sourcePath, err.Error())
				retriesLeft--
				continue retryLoop
			}
			source.Buffer.Write(dataBuf[:n])
			select {
			case metaFrame := <-mfChannel:
				meta, err := metaFrame.ParseMeta()
				if err != nil {
					logger.Errorf("SOURCE \"%s\": error parsing metadata: %s", sourcePath, err.Error())
				} else {
					source.metaChannel <- meta
					logger.Noticef("SOURCE \"%s\": got metadata %v", sourcePath, meta)
				}
			default:
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
	for {
		select {
		case meta := <-s.metaChannel:
			s.currentMeta = meta
			s.listeners.Iter(func(lr *Listener) {
				select {
				case lr.metaBuffer <- meta:
				default:
				}
			})
		}
	}
}
