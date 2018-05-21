package icy

import (
	"errors"
	"fmt"
	"io"
)

type (
	MetaFrame []byte
	MetaData  map[string]string

	IcyReader struct {
		source       io.Reader
		metaInterval int
		metaPointer  int
		frameChannel chan MetaFrame
	}
)

// FSM states
const (
	StateReadKey = iota
	StateWaitForValue
	StateReadValue
	StateReadQuotedValue
	StateWaitSemicolon
)

func NewReader(src io.Reader, metaInterval int, c chan MetaFrame) *IcyReader {
	return &IcyReader{src, metaInterval, 0, c}
}

func (p *IcyReader) Read(buf []byte) (int, error) {
	if p.metaInterval > 0 && p.metaPointer+len(buf) > p.metaInterval {
		bytesToRead := p.metaInterval - p.metaPointer

		n, err := io.ReadFull(p.source, buf[:bytesToRead])
		if err != nil {
			return n, err
		}

		frame := make(MetaFrame, 4096)

		m, err := io.ReadFull(p.source, frame[:1])
		if err != nil {
			return n, err
		}

		frameLength := int(frame[0]) * 16
		if frameLength > 0 {
			m, err = io.ReadFull(p.source, frame[1:frameLength+1])
			if err != nil {
				return n, err
			}
			if m != frameLength {
				return n, errors.New("error reading metaframe")
			}
			frame = frame[:frameLength+1]
			select {
			case p.frameChannel <- frame:
			default:
			}
		}
		m, err = io.ReadFull(p.source, buf[bytesToRead:])
		if err != nil {
			return n, err
		}
		p.metaPointer = m
		return m + n, err
	}

	n, err := p.source.Read(buf)
	p.metaPointer += n
	return n, err
}

func (mf MetaFrame) Inspect() string {
	return fmt.Sprintf("<MetaFrame fullLen=%d expDataLen=%d(%d) realDataLen=%d>%s</MetaFrame>", len(mf), mf[0], mf[0]*16, len(mf[1:]), string(mf[1:]))
}

// ParseMeta returns meta as a map[string]string
func (mf MetaFrame) ParseMeta() (MetaData, error) {
	i := len(mf) - 1
	for mf[i] == 0 {
		i--
		if i < 0 {
			break
		}
	}
	metaString := string(mf[:i+1])
	result := make(MetaData)
	i = 0
	k := ""
	v := ""
	state := StateReadKey
parseLoop:
	for {
		if i >= len(metaString) && state != StateReadKey {
			return make(MetaData), errors.New("unexpected end of metadata")
		}
		switch state {
		case StateReadKey:
			if i >= len(metaString) {
				break parseLoop
			}
			if metaString[i] == '=' {
				i++
				if i >= len(metaString) {
					return make(MetaData), errors.New("unexpected end of metadata")
				}
				if metaString[i] == '\'' {
					state = StateReadQuotedValue
					i++
				} else {
					state = StateReadValue
				}
			} else {
				k += string(metaString[i])
				i++
			}
		case StateReadValue:
			if metaString[i] == ';' {
				result[k] = v
				k = ""
				v = ""
				state = StateReadKey
			} else {
				v += string(metaString[i])
			}
			i++
		case StateReadQuotedValue:
			if metaString[i] == '\'' {
				result[k] = v
				k = ""
				v = ""
				state = StateWaitSemicolon
			} else {
				v += string(metaString[i])
			}
			i++
		case StateWaitSemicolon:
			if metaString[i] != ';' {
				return make(MetaData), errors.New("semicolon expected")
			} else {
				state = StateReadKey
				i++
			}
		}
	}
	return result, nil
}

func (md *MetaData) Render() MetaFrame {
	if len(*md) == 0 {
		return make(MetaFrame, 1)
	}

	metaString := ""
	for key, value := range *md {
		metaString += fmt.Sprintf("%s='%s';", key, value)
	}

	metaLength := len(metaString) / 16
	if len(metaString)%16 != 0 {
		metaLength++
	}

	result := make([]byte, metaLength*16+1)
	result[0] = byte(metaLength)
	for i, sym := range metaString {
		result[i+1] = byte(sym)
	}
	return MetaFrame(result)
}
