package cast

import (
	"errors"
	"fmt"
	"icy"
	"mpeg"
	"net/http"
	"os"
	"sync"
	"time"
)

type (
	Listener struct {
		responseWriter http.ResponseWriter
		request        *http.Request
		sourcePath     string
		joined         time.Time
		metaBuffer     chan icy.MetaData
		currentMeta    icy.MetaFrame
		key            string
	}

	ListenerSlice struct {
		sync.Mutex
		listeners []*Listener
	}
)

const (
	listenerBufferSize  = 4096
	defaultMetaInterval = 16000
)

func NewListenerSlice(allocateSize int) *ListenerSlice {
	return &ListenerSlice{
		listeners: make([]*Listener, 0, allocateSize),
	}
}

func (ls *ListenerSlice) Add(lr *Listener) {
	ls.Lock()
	defer ls.Unlock()
	ls.listeners = append(ls.listeners, lr)
}

func (ls *ListenerSlice) Remove(lr *Listener) int {
	ls.Lock()
	defer ls.Unlock()
	for i, listener := range ls.listeners {
		if listener == lr {
			ls.listeners = append(ls.listeners[:i], ls.listeners[i+1:]...)
			return i
		}
	}
	return -1
}

func (ls *ListenerSlice) Iter(fn func(*Listener)) {
	ls.Lock()
	defer ls.Unlock()
	for _, listener := range ls.listeners {
		fn(listener)
	}
}

func NewListener(rw http.ResponseWriter, req *http.Request, sourcePath string) *Listener {
	return &Listener{
		rw,
		req,
		sourcePath,
		time.Now(),
		make(chan icy.MetaData, 1),
		make([]byte, 1),
		fmt.Sprintf("%s:%s", req.RemoteAddr, sourcePath),
	}
}

func handleListener(rw http.ResponseWriter, req *http.Request) {
	sourcePath := req.URL.RequestURI()

	source, found := sourcesPathMap[sourcePath]
	if !found || !source.active {
		http.Error(rw, "Source not found", http.StatusNotFound)
		return
	}

	lr := NewListener(rw, req, sourcePath)
	source.listeners.Add(lr)
	logger.Noticef("SOURCE \"%s\": listener %s has joined", source.config.Path, lr.key)

	rw.Header().Set("Content-Type", "audio/mpeg")
	rw.Header().Set("icy-br", fmt.Sprintf("%d", source.config.Stream.Bitrate))
	rw.Header().Set("ice-audio-info", fmt.Sprintf("bitrate=%d", source.config.Stream.Bitrate))
	rw.Header().Set("icy-description", source.config.Stream.Description)
	rw.Header().Set("icy-name", source.config.Stream.Name)
	rw.Header().Set("icy-genre", source.config.Stream.Genre)
	if source.config.Stream.Public {
		rw.Header().Set("icy-pub", "1")
	} else {
		rw.Header().Set("icy-pub", "0")
	}
	rw.Header().Set("icy-url", source.config.Stream.URL)

	metaInt := 0
	metaPtr := 0
	metaRequested := req.Header.Get("Icy-MetaData")
	if metaRequested == "1" {
		metaInt = defaultMetaInterval
	}
	if metaInt != 0 {
		rw.Header().Set("icy-metaint", fmt.Sprintf("%d", metaInt))
	}

	rw.WriteHeader(200)

	synced := false
	srcReader := source.Buffer.NewReader(source.Buffer.MidPoint())
	buf := make([]byte, listenerBufferSize)
	lr.currentMeta = source.currentMeta.Render()
	var meta icy.MetaData
	var err error
	var n int
	var chunk []byte

	for {

		n, err = srcReader.Read(buf)

		if err != nil {
			logger.Errorf("error reading source buffer: %s", err.Error())
			break
		}

		if n == 0 {
			time.Sleep(30 * time.Millisecond)
			continue
		}

		if !synced {
			chunk, err = frameSync(buf[:n])
			if err != nil {
				logger.Errorf("error framesyncing")
				break
			}
			synced = true
		} else {
			chunk = buf[:n]
		}

		if metaInt > 0 {
			if metaPtr+len(chunk) > metaInt {
				nch := make([]byte, len(chunk)+len(lr.currentMeta))
				insertPos := metaInt - metaPtr
				metaFrameLen := len(lr.currentMeta)

				copy(nch[:insertPos], chunk[:insertPos])
				copy(nch[insertPos:insertPos+metaFrameLen], lr.currentMeta)
				copy(nch[insertPos+metaFrameLen:], chunk[insertPos:])

				if metaFrameLen != 1 {
					lr.currentMeta = make(icy.MetaFrame, 1)
				}

				metaPtr = len(chunk) - insertPos
				chunk = nch
			} else {
				metaPtr += len(chunk)
			}
		}

		n, err = rw.Write(chunk)
		if err != nil {
			logger.Noticef("SOURCE \"%s\": listener %s has gone", source.config.Path, lr.key)
			break
		}

		select {
		case meta = <-lr.metaBuffer:
			lr.currentMeta = meta.Render()
		default:
		}
	}
	source.listeners.Remove(lr)
}

func frameSync(chunk []byte) ([]byte, error) {
	for i := 0; i < len(chunk)-4; i++ {
		if mpeg.FrameHeaderValid(chunk[i:]) {
			return chunk[i:], nil
		}
	}
	return chunk, errors.New("no valid frame found")
}

func dumpChunk(chunk []byte, filename string) {
	f, err := os.Create(filename)
	if err != nil {
		return
	}
	defer f.Close()
	for _, b := range chunk {
		f.Write([]byte(fmt.Sprintf("%02X\n", b)))
	}
}
