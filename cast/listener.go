package cast

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/viert/endless"
	"github.com/viert/flamecast/configreader"
	"github.com/viert/flamecast/icy"
	"github.com/viert/flamecast/mpeg"
)

type (
	Listener struct {
		responseWriter   http.ResponseWriter
		request          *http.Request
		sourcePath       string
		joined           time.Time
		currentMetaFrame *icy.MetaFrame
		key              string
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

var (
	zeroMetaFrame = icy.MetaFrame{0}
)

func newListenerSlice(allocateSize int) *ListenerSlice {
	return &ListenerSlice{
		listeners: make([]*Listener, 0, allocateSize),
	}
}

func (ls *ListenerSlice) add(lr *Listener) {
	ls.Lock()
	defer ls.Unlock()
	stats.ListenersCount++
	ls.listeners = append(ls.listeners, lr)
}

func (ls *ListenerSlice) remove(lr *Listener) int {
	ls.Lock()
	defer ls.Unlock()

	for i, listener := range ls.listeners {
		if listener == lr {
			ls.listeners = append(ls.listeners[:i], ls.listeners[i+1:]...)
			stats.ListenersCount--
			return i
		}
	}
	return -1
}

func (ls *ListenerSlice) iter(fn func(*Listener)) {
	ls.Lock()
	defer ls.Unlock()
	for _, listener := range ls.listeners {
		fn(listener)
	}
}

// NewListener creates a new listener
func NewListener(rw http.ResponseWriter, req *http.Request, sourcePath string) *Listener {
	return &Listener{
		rw,
		req,
		sourcePath,
		time.Now(),
		&zeroMetaFrame,
		fmt.Sprintf("%s:%s", req.RemoteAddr, sourcePath),
	}
}

func extractToken(req *http.Request) string {
	token := req.URL.Query().Get("token")
	if token != "" {
		return token
	}
	token = req.Header.Get("X-Flamecast-Token")
	if token != "" {
		return token
	}
	auth := req.Header.Get("Authorization")
	if auth != "" {
		if auth[:6] == "Token " {
			token = strings.TrimSpace(auth[6:])
		}
	}
	return token
}

func checkToken(token string, lr *Listener, checkURL *url.URL) bool {
	logger.Debugf("Checking token \"%s\"", token)
	client := new(http.Client)
	body := fmt.Sprintf(`{
	"token": "%s",
	"listener": {
		"remote_addr": "%s",
		"key": "%s"
	},
	"source": {
		"path": "%s"
	}
}`, token, lr.request.RemoteAddr, lr.key, lr.sourcePath)

	req, err := http.NewRequest("POST", checkURL.String(), strings.NewReader(body))
	if err != nil {
		logger.Errorf("error creating request for token check: %s", err.Error())
		return false
	}
	req.Header.Add("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		logger.Errorf("error getting response for token check: %s", err.Error())
		return false
	}
	if resp.StatusCode != http.StatusOK {
		return false
	}

	checkResponse := resp.Header.Get("flamecast-auth-user")
	if checkResponse == "" {
		checkResponse = resp.Header.Get("icecast-auth-user")
	}
	if checkResponse == "1" {
		return true
	}
	return false
}

func listenerNotify(lr *Listener, notifyUrl *url.URL, notifyType string) {
	if notifyUrl == nil {
		return
	}

	q := notifyUrl.Query()
	q.Add("source", lr.sourcePath)
	q.Add("listener", lr.key)
	req, err := http.NewRequest("GET", notifyUrl.String(), nil)
	if err != nil {
		logger.Errorf("Error creating listener notify request: %s", err)
		return
	}

	go func() {
		cli := &http.Client{}
		resp, err := cli.Do(req)
		if err != nil {
			logger.Errorf("Error requesting listener notify url: %s", err)
		} else {
			logger.Debugf("Listener %s %s notify, status_code = %d", lr.key, notifyType, resp.StatusCode)
		}
	}()
}

func handleListener(rw http.ResponseWriter, req *http.Request) {

	sourcePath := req.URL.Path
	source, found := sourcesPathMap[sourcePath]
	if !found {
		http.Error(rw, "Source not found", http.StatusNotFound)
		return
	}
	altSource, hasAlt := sourcesPathMap[source.config.FallbackPath]

	// Setting up listener
	lr := NewListener(rw, req, sourcePath)
	if source.config.BroadcastAuthType == configreader.BroadcastAuthTypeToken {
		token := extractToken(req)
		if token == "" || !checkToken(token, lr, source.config.BroadcastAuthTokenCheckURL) {
			if token == "" {
				logger.Errorf("Listener %s at source %s has no token, rejecting", lr.key, sourcePath)
			} else {
				logger.Errorf("Listener %s at source %s has invalid token \"%s\", rejecting", lr.key, sourcePath, token)
			}
			http.Error(rw, "Authentication failed", http.StatusUnauthorized)
			return
		}
	}

	stats.ListenerConnections++

	logger.Noticef("SOURCE \"%s\": listener %s has joined", source.config.Path, lr.key)
	listenerNotify(lr, source.config.BroadcastNotifyEnterURL, "enter")

	// Setting up source reader
	var isAlt bool
	var srcReader *endless.Reader
	var currentSource *Source
	var synced = false
	var metaFrame icy.MetaFrame
	var err error
	var n int
	var chunk []byte
	var buf = make([]byte, listenerBufferSize)
	logger.Debugf("Allocated listener buffer, size=%d", listenerBufferSize)

	if !source.active {
		if !hasAlt || !altSource.active {
			http.Error(rw, "source not found", http.StatusNotFound)
			logger.Errorf("SOURCE \"%s\": listener %s dropped as source is not active and there's no alternative",
				sourcePath, lr.key)
			return
		}
		altSource.listeners.add(lr)
		logger.Noticef("SOURCE \"%s\": listener %s started with fallback stream", sourcePath, lr.key)
		srcReader = altSource.Buffer.NewReader(altSource.Buffer.MidPoint())
		isAlt = true
	} else {
		source.listeners.add(lr)
		srcReader = source.Buffer.NewReader(source.Buffer.MidPoint())
		isAlt = false
	}

	// Setting up listener headers
	rw.Header().Set("Content-Type", "audio/mpeg")
	rw.Header().Set("icy-br", fmt.Sprintf("%d", source.config.Stream.Bitrate))
	rw.Header().Set("ice-audio-info", source.config.Stream.AudioInfo)
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
		logger.Debugf("Icy metadata requested, interval set to %d", defaultMetaInterval)
	}
	if metaInt != 0 {
		rw.Header().Set("icy-metaint", fmt.Sprintf("%d", metaInt))
	}
	rw.WriteHeader(200)

	writeChunk := func(chunk []byte) bool {
		_, err := rw.Write(chunk)
		if err != nil {
			logger.Noticef("SOURCE \"%s\": listener %s has gone", source.config.Path, lr.key)
			return false
		}
		return true
	}

	for {

		if isAlt {
			if source.active {
				logger.Noticef("SOURCE \"%s\": source got active, moving listener %s back from fallback",
					sourcePath, lr.key)
				srcReader = source.Buffer.NewReader(source.Buffer.MidPoint())
				synced = false
				isAlt = false
				altSource.listeners.remove(lr)
				source.listeners.add(lr)
			} else if !altSource.active {
				altSource.listeners.remove(lr)
				logger.Errorf("SOURCE \"%s\": no more active sources for listener %s, giving up", sourcePath, lr.key)
				break
			}
		} else {
			if !source.active {
				if altSource == nil {
					logger.Noticef("SOURCE \"%s\": source has stopped, no alternative source is defined, giving up with listener %s",
						sourcePath, lr.key)
					break
				} else {
					logger.Noticef("SOURCE \"%s\": source has stopped, moving listener %s to fallback",
						sourcePath, lr.key)
					source.listeners.remove(lr)
					altSource.listeners.add(lr)
				}
				srcReader = altSource.Buffer.NewReader(altSource.Buffer.MidPoint())
				synced = false
				isAlt = true
			}
		}

		n, err = srcReader.Read(buf)

		if err != nil {
			logger.Errorf("SOURCE \"%s\": error reading source buffer: %s", sourcePath, err.Error())
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

				if isAlt {
					currentSource = altSource
				} else {
					currentSource = source
				}

				if lr.currentMetaFrame != currentSource.currentMetaFrame {
					lr.currentMetaFrame = currentSource.currentMetaFrame
					metaFrame = *currentSource.currentMetaFrame
				} else {
					metaFrame = zeroMetaFrame
				}

				// nch := make([]byte, len(chunk)+len(metaFrame))
				insertPos := metaInt - metaPtr

				if !writeChunk(chunk[:insertPos]) {
					break
				}

				if !writeChunk(metaFrame) {
					break
				}

				if !writeChunk(chunk[insertPos:]) {
					break
				}

				metaPtr = len(chunk) - insertPos
			} else {
				metaPtr += len(chunk)
				if !writeChunk(chunk) {
					break
				}
			}
		} else {
			if !writeChunk(chunk) {
				break
			}
		}

	}
	// removing listeners from all sources whereever he may currently be
	source.listeners.remove(lr)
	if hasAlt {
		altSource.listeners.remove(lr)
	}
	listenerNotify(lr, source.config.BroadcastNotifyLeaveURL, "leave")
}

func frameSync(chunk []byte) ([]byte, error) {
	for i := 0; i < len(chunk)-4; i++ {
		if mpeg.FrameHeaderValid(chunk[i:]) {
			return chunk[i:], nil
		}
	}
	return chunk, errors.New("no valid frame found")
}
