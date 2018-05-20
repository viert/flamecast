package mpeg

import (
	"errors"
)

// FSM states
const (
	StateReadKey = iota
	StateWaitForValue
	StateReadValue
	StateReadQuotedValue
	StateWaitSemicolon
)

func ParseMeta(data []byte) (map[string]string, error) {
	i := len(data) - 1
	for data[i] == 0 {
		i--
		if i < 0 {
			break
		}
	}
	metaString := string(data[:i+1])
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
				if data[i] == '\'' {
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
