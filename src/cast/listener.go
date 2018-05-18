package cast

import (
	"fmt"
	"github.com/tcolgate/mp3"
	"net/http"
	"sync"
	"time"
)

const (
	FrameBufferInitSize       = 128
	FrameBufferPercentMin     = 0.5
	FrameBufferPercentMaxWait = 0.7
	TimeSyncIterMin           = 3
	TimeSyncIterMax           = 5
	TimeSyncInitialValue      = 100 * time.Millisecond
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
		make(chan *mp3.Frame, FrameBufferInitSize),
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
	logger.Noticef("Listener %s has joined", lr.key)

	rw.Header().Set("Content-Type", "audio/mpeg")
	rw.WriteHeader(200)

	// Initial buffering
	wTime := timeSync(TimeSyncInitialValue, lr.frameBuffer)

	for {
		wTime = timeSync(wTime, lr.frameBuffer)
		frame := <-lr.frameBuffer

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
	source.listeners.Remove(lr)
}

func timeSync(waitTime time.Duration, frameBuffer chan *mp3.Frame) time.Duration {
	// If buffer gets FrameBufferPercentMin full wait until it gets FrameBufferPercentMaxWait full
	if float64(len(frameBuffer)) < float64(cap(frameBuffer))*FrameBufferPercentMin {
		iter := 0
		for float64(len(frameBuffer)) < float64(cap(frameBuffer))*FrameBufferPercentMaxWait {
			fmt.Println(len(frameBuffer), cap(frameBuffer), waitTime, iter)
			time.Sleep(waitTime)
			iter++
		}

		// Tweaking sleep time
		if iter < TimeSyncIterMin {
			return waitTime - 10*time.Millisecond
		} else if iter > TimeSyncIterMax {
			return waitTime + 10*time.Millisecond
		}
	}
	return waitTime
}
