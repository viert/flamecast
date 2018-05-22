package cast

import (
	"configreader"
	"github.com/viert/endless"
	"icy"
	"net/http"
	"strconv"
)

const (
	InitialListenersCount    = 256
	DataBufferSize           = 4096
	EndlessSize              = 512 * 1024
	BlocksWrittenUntilActive = 4
)

type (
	// Source is the main source holder with configuration, buffers, metadata, listeners etc.
	Source struct {
		config           *configreader.SourceConfig
		Buffer           *endless.Endless
		currentMeta      icy.MetaData
		currentMetaFrame *icy.MetaFrame
		listeners        *ListenerSlice
		active           bool
	}
)

func NewSource(config *configreader.SourceConfig) *Source {
	return &Source{
		config,
		endless.NewEndless(EndlessSize),
		make(icy.MetaData),
		&icy.MetaFrame{0},
		NewListenerSlice(512),
		false,
	}
}

func pullSource(source *Source) {
	retriesLeft := PULL_RETRIES_MAX

	sourceURL := source.config.SourcePullUrl.String()
	sourcePath := source.config.Path

	cli := new(http.Client)

retryLoop:
	for retriesLeft > 0 {
		req, err := http.NewRequest("GET", sourceURL, nil)
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

		var metaInterval int64
		miString := resp.Header.Get("icy-metaint")
		if miString != "" {
			metaInterval, _ = strconv.ParseInt(miString, 10, 64)
		}

		logger.Noticef("SOURCE \"%s\": source puller connected", sourcePath)

		mfChannel := make(chan icy.MetaFrame, 1)
		reader := icy.NewReader(resp.Body, int(metaInterval), mfChannel)
		dataBuf := make([]byte, DataBufferSize)

		iterations := 0

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
					source.currentMeta = meta
					source.currentMetaFrame = &metaFrame
					logger.Noticef("SOURCE \"%s\": got metadata %v", sourcePath, meta)
				}
			default:
			}

			if !source.active {
				iterations++
				if iterations == BlocksWrittenUntilActive {
					logger.Noticef("SOURCE \"%s\": source buffer filled, source is now active", sourcePath)
					source.active = true
				}
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
