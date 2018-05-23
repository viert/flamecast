package cast

import (
	"configreader"
	"github.com/viert/endless"
	"icy"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	BlocksWrittenUntilActive = 4
	DataBufferSize           = 4096
	EndlessSize              = 16 * 4096
	InitialListenersCount    = 256
	PullRetriesMax           = 5
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

		Started     time.Time
		ContentType string
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
		time.Now(),
		"audio/mpeg",
	}
}

func pullSource(source *Source) {
	retriesLeft := PullRetriesMax

	sourceURL := source.config.SourcePullURL.String()
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

		readIceHeaders(source, resp.Header)

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
					source.Started = time.Now()
				}
			}
		}
	}
	source.active = false
}

func pushSource(rw http.ResponseWriter, req *http.Request) {

	sourcePath := req.URL.RequestURI()
	source, found := sourcesPathMap[sourcePath]
	if !found {
		http.Error(rw, "Source not found", http.StatusNotFound)
		return
	}

	if source.active {
		logger.Errorf("SOURCE \"%s\": tried to feed already active source", sourcePath)
		http.Error(rw, "Source is already streaming", http.StatusConflict)
		return
	}

	if !checkSourceAuth(source, req) {
		logger.Errorf("SOURCE \"%s\": Feeder authorization failed", sourcePath)
		http.Error(rw, "Source authorization failed", http.StatusUnauthorized)
		return
	}

	readIceHeaders(source, req.Header)
	logger.Noticef("SOURCE \"%s\": feeder accepted", sourcePath)

	hj, ok := rw.(http.Hijacker)
	if !ok {
		logger.Errorf("SOURCE \"%s\": hijacking failed", sourcePath)
		http.Error(rw, "hijacking failed", http.StatusInternalServerError)
		return
	}
	conn, bufrw, err := hj.Hijack()
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	defer conn.Close()

	bufrw.WriteString("HTTP/1.0 200 OK\r\n\r\n")
	bufrw.Flush()

	iterations := 0
	dataBuf := make([]byte, DataBufferSize)

	for {
		n, err := bufrw.Read(dataBuf)
		if err != nil {
			logger.Noticef("SOURCE \"%s\": feeder has disconnected", sourcePath)
			source.active = false
			break
		}
		source.Buffer.Write(dataBuf[:n])
		if !source.active {
			iterations++
			if iterations == BlocksWrittenUntilActive {
				logger.Noticef("SOURCE \"%s\": source buffer filled, source is now active", sourcePath)
				source.active = true
				source.Started = time.Now()
			}
		}
	}
}

func sourceHandler(rw http.ResponseWriter, req *http.Request) {
	switch req.Method {
	// case "PUT":
	// 	fallthrough
	case "SOURCE":
		pushSource(rw, req)

	case "GET":
		handleListener(rw, req)
	}
}

func setSourceMetadata(s *Source, md icy.MetaData) {
	frame := md.Render()
	s.currentMeta = md
	s.currentMetaFrame = &frame
	logger.Noticef("SOURCE \"%s\": got metadata %v", s.config.Path, md)
}

func checkSourceAuth(s *Source, req *http.Request) bool {
	token := req.Header.Get("Authorization")
	return token == "Basic "+s.config.SourceAuthToken
}

func readIceHeaders(s *Source, hdr http.Header) {
	name := hdr.Get("Ice-Name")
	if name != "" {
		s.config.Name = name
		s.config.Stream.Name = name
	}

	description := hdr.Get("Ice-Description")
	if description != "" {
		s.config.Stream.Description = description
	}

	genre := hdr.Get("Ice-Genre")
	if genre != "" {
		s.config.Stream.Genre = genre
	}

	url := hdr.Get("Ice-Url")
	if url != "" {
		s.config.Stream.URL = url
	}

	public := hdr.Get("Ice-Public")
	if public != "" {
		public = strings.ToLower(public)
		if public == "0" || public == "false" || public == "no" {
			s.config.Stream.Public = false
		} else {
			s.config.Stream.Public = true
		}
	}
}
