package cast

import (
	"fmt"
	"github.com/tcolgate/mp3"
	"net/http"
	"sync"
	"time"
)

const (
	frameBufferInitSize       = 128
	frameBufferPercentMin     = 0.5
	frameBufferPercentMaxWait = 0.7
	timeSyncIterMin           = 3
	timeSyncIterMax           = 5
	timeSyncInitialValue      = 100 * time.Millisecond
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

type ListenerSlice struct {
	sync.Mutex
	listeners []*Listener
}

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
		make(chan *mp3.Frame, frameBufferInitSize),
		make([]byte, 8192),
		fmt.Sprintf("%s:%s", req.RemoteAddr, sourcePath),
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
	source.listeners.Add(lr)
	logger.Noticef("SOURCE \"%s\": listener %s has joined", source.config.Path, lr.key)

	rw.Header().Set("Content-Type", "audio/mpeg")
	rw.WriteHeader(200)

	// Initial buffering
	logger.Debugf("SOURCE \"%s\": initial buffering stream for listener %s", source.config.Path, lr.key)
	wTime := timeSync(timeSyncInitialValue, lr.frameBuffer)
	logger.Debugf("SOURCE \"%s\": initial buffering complete for listener %s", source.config.Path, lr.key)

	for {
		wTime = timeSync(wTime, lr.frameBuffer)
		frame := <-lr.frameBuffer

		n, err := frame.Reader().Read(lr.dataBuffer)
		if err != nil {
			logger.Errorf("SOURCE \"%s\": error reading data from frame: %s", source.config.Path, err)
			break
		}
		_, err = rw.Write(lr.dataBuffer[:n])
		if err != nil {
			logger.Noticef("SOURCE \"%s\": listener %s has gone", source.config.Path, lr.key)
			break
		}
	}
	source.listeners.Remove(lr)
}

func timeSync(waitTime time.Duration, frameBuffer chan *mp3.Frame) time.Duration {
	// If buffer gets FrameBufferPercentMin full wait until it gets FrameBufferPercentMaxWait full
	if float64(len(frameBuffer)) < float64(cap(frameBuffer))*frameBufferPercentMin {
		iter := 0
		for float64(len(frameBuffer)) < float64(cap(frameBuffer))*frameBufferPercentMaxWait {
			time.Sleep(waitTime)
			iter++
		}

		// Tweaking sleep time
		if iter < timeSyncIterMin {
			return waitTime - 10*time.Millisecond
		} else if iter > timeSyncIterMax {
			return waitTime + 10*time.Millisecond
		}
	}
	return waitTime
}
