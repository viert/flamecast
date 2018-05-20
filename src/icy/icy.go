package icy

import (
	"errors"
	"io"
)

type (
	MetaFrame []byte

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
		// Cut the metaframe
		bytesToRead := p.metaInterval - p.metaPointer
		n, err := p.source.Read(buf[:bytesToRead])
		if err != nil {
			return n, err
		}
		frame := make(MetaFrame, 4096)
		m, err := p.source.Read(frame[:1])
		if err != nil {
			return n, err
		}
		if m != 1 {
			return n, errors.New("error reading metaframe header")
		}
		frameLength := int(frame[0]) * 16
		if frameLength > 0 {
			m, err = p.source.Read(frame[:frameLength])
			if err != nil {
				return n, err
			}
			if m != frameLength {
				return n, errors.New("error reading metaframe")
			}
			frame = frame[:frameLength]

			select {
			case p.frameChannel <- frame:
			default:
			}
		}
		m, err = p.source.Read(buf[bytesToRead:])
		p.metaPointer = m
		return n + m, err
	}

	n, err := p.source.Read(buf)
	p.metaPointer += n
	return n, err
}

// ParseMeta returns meta as a map[string]string
func (mf MetaFrame) ParseMeta() (map[string]string, error) {
	i := len(mf) - 1
	for mf[i] == 0 {
		i--
		if i < 0 {
			break
		}
	}
	metaString := string(mf[:i+1])
	result := make(map[string]string)
	i = 0
	k := ""
	v := ""
	state := StateReadKey
parseLoop:
	for {
		if i >= len(metaString) && state != StateReadKey {
			return result, errors.New("unexpected end of metadata")
		}
		switch state {
		case StateReadKey:
			if i >= len(metaString) {
				break parseLoop
			}
			if metaString[i] == '=' {
				i++
				if i >= len(metaString) {
					return result, errors.New("unexpected end of metadata")
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
				return result, errors.New("semicolon expected")
			} else {
				state = StateReadKey
				i++
			}
		}
	}
	return result, nil
}
