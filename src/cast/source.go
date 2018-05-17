package cast

import (
	"configreader"
	"fmt"
	"github.com/tcolgate/mp3"
	"net/http"
	"time"
)

const (
	InitialListenersCount = 256
)

type Listener struct {
	responseWriter http.ResponseWriter
	request        *http.Request
	sourcePath     string
	joined         time.Time
	frameBuffer    chan *mp3.Frame
	dataBuffer     []byte
	key            string
}

type Source struct {
	config        *configreader.SourceConfig
	inputChannel  chan *mp3.Frame
	listeners     []*Listener
	active        bool
	addChannel    chan *Listener
	removeChannel chan *Listener
}

func NewListener(rw http.ResponseWriter, req *http.Request, sourcePath string) *Listener {
	return &Listener{
		rw,
		req,
		sourcePath,
		time.Now(),
		make(chan *mp3.Frame, 5120),
		make([]byte, 8192),
		fmt.Sprintf("%s:%s", req.RemoteAddr, sourcePath),
	}
}

func NewSource(config *configreader.SourceConfig) *Source {
	return &Source{
		config,
		make(chan *mp3.Frame, 5120),
		make([]*Listener, 0, InitialListenersCount),
		false,
		make(chan *Listener),
		make(chan *Listener),
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

func handleListener(rw http.ResponseWriter, req *http.Request) {
	sourcePath := req.URL.RequestURI()

	source, found := sourcesPathMap[sourcePath]
	if !found {
		http.Error(rw, "Source not found", http.StatusNotFound)
		return
	}

	lr := NewListener(rw, req, sourcePath)
	source.addListener(lr)
	logger.Noticef("Listener %s has joined", lr.key)

	rw.Header().Set("Content-Type", "audio/mpeg")
	rw.WriteHeader(200)

	for frame := range lr.frameBuffer {
		n, err := frame.Reader().Read(lr.dataBuffer)
		if err != nil {
			logger.Errorf("Error reading data from frame: %s", err)
			break
		}
		_, err = rw.Write(lr.dataBuffer[:n])
		if err != nil {
			logger.Noticef("Listener %s has gone", lr.key)
			break
		}
	}
	source.removeListener(lr)
}

func (s *Source) addListener(lr *Listener) {
	s.addChannel <- lr
}

func (s *Source) removeListener(lr *Listener) {
	s.removeChannel <- lr
}

func (s *Source) multiplex() {
	var lr *Listener
	for frame := range s.inputChannel {
		for _, lr = range s.listeners {
			// ACHTUNG! SHOULD BE NON BLOCKING
			// (as we don't care about listeners)
			select {
			case lr.frameBuffer <- frame:
			default:
			}
		}
		select {
		case lr = <-s.addChannel:
			logger.Notice("Adding listener to source")
			s.listeners = append(s.listeners, lr)
		case lr = <-s.removeChannel:
			n := -1
			for i := 0; i < len(s.listeners); i++ {
				if lr.key == s.listeners[i].key {
					n = i
					break
				}
			}
			if n >= 0 {
				s.listeners = append(s.listeners[:n], s.listeners[n+1:]...)
			}
		default:
		}
	}
}
