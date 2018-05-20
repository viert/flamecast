package mpeg

import (
	"fmt"
	"io"
	"os"
)

type (
	Parser struct {
		source       io.Reader
		bytesRead    uint64
		buffer       []byte
		metaInterval int
		metaPointer  int
		dump         *os.File
	}
	FrameType int
)

const (
	// FrameBufSize is the initial parser buffer size
	FrameBufSize = 4096
	// HeaderLength is the length of a FrameHeader
	HeaderLength = 4
)

// FrameType values
const (
	FrameTypeNone FrameType = iota
	FrameTypeMP3
	FrameTypeMeta
)

// NewParser returns a new stream parser
func NewParser(reader io.Reader, metaInterval int) *Parser {
	f, _ := os.Create("frame.dump")
	return &Parser{reader, 0, make([]byte, 0, FrameBufSize), metaInterval, 0, f}
}

func (p *Parser) fillUp(l int) error {
	// Fills up buffer up to at least L elements from the Source
	bytesToRead := l - len(p.buffer)
	if bytesToRead < 1 {
		// enough of bytes in buffer, nothing to read
		return nil
	}

	currentLength := len(p.buffer)

	if cap(p.buffer) < l {
		p.buffer = append(p.buffer, make([]byte, l-currentLength)...)
	}

	p.buffer = p.buffer[:l]
	_, err := io.ReadFull(p.source, p.buffer[currentLength:])
	p.dump.Write(p.buffer[currentLength:])
	return err
}

func (p *Parser) Iter() ([]byte, uint64, FrameType) {
	for {
		if len(p.buffer) < 4 {
			err := p.fillUp(FrameBufSize)
			if err != nil {
				return make([]byte, 0), p.bytesRead, FrameTypeNone
			}
		}

		if p.metaInterval != 0 && p.metaPointer != 0 && p.metaPointer == p.metaInterval {
			metaLength := int(p.buffer[0]) * 16
			p.fillUp(metaLength + 1)
			frameData := p.buffer[1 : metaLength+1]
			pos := p.bytesRead + 1

			p.bytesRead += uint64(metaLength + 1)
			p.metaPointer = 0
			p.buffer = p.buffer[metaLength+1:]

			return frameData, pos, FrameTypeMeta
		}

		if p.buffer[0] != 0xFF || p.buffer[1]&0xE0 != 0xE0 {
			p.bytesRead++
			p.metaPointer++
			p.buffer = p.buffer[1:]
			continue
		} else {
			hdr := FrameHeader(p.buffer)
			if hdr.Emphasis() != EmphasisReserved &&
				hdr.Layer() != LayerReserved &&
				hdr.Version() != VersionMPEGReserved &&
				hdr.SampleRate() != SampleRateInvalid &&
				hdr.BitRate() != BitRateInvalid {

				frameSize := hdr.FrameSize()
				p.fillUp(frameSize)

				pos := p.bytesRead
				var res []byte
				// Checking if the frame is broken apart by a sudden metaframe
				if p.metaInterval != 0 && p.metaPointer+frameSize > p.metaInterval {
					frameSplitPos := p.metaInterval - p.metaPointer
					metaFrameLength := int(p.buffer[frameSplitPos]) * 16

					// +1 is for the byte storing metaframe length data
					p.fillUp(frameSize + metaFrameLength + 1)

					remainingFrameStart := frameSplitPos + 1 + metaFrameLength
					remainingFrameLength := frameSize - frameSplitPos

					p.metaPointer = remainingFrameLength

					res = make([]byte, frameSize)
					copy(res[:frameSplitPos], p.buffer[:frameSplitPos])
					copy(res[frameSplitPos:], p.buffer[remainingFrameStart:remainingFrameStart+remainingFrameLength])

					if metaFrameLength > 0 {
						fmt.Printf("Cut by meta frame length %d at %d\n", metaFrameLength, p.bytesRead+uint64(frameSplitPos))
						metaFrame := p.buffer[frameSplitPos+1 : frameSplitPos+1+metaFrameLength]
						dumpdata(metaFrame)
					}

					p.buffer = p.buffer[frameSize+metaFrameLength+1:]
				} else {
					res = p.buffer[:frameSize]
					p.metaPointer += frameSize
					p.buffer = p.buffer[frameSize:]
				}

				p.bytesRead += uint64(frameSize)

				return res, pos, FrameTypeMP3
			} else {
				p.bytesRead++
				p.metaPointer++
				p.buffer = p.buffer[1:]
			}
		}
	}
}

func dumpdata(data []byte) {
	hex := ""
	chrs := ""
	for i := 0; i < len(data); i++ {
		if i != 0 && i%20 == 0 {
			fmt.Println(hex + chrs)
			hex = ""
			chrs = ""
		}
		hex += fmt.Sprintf("%02x ", data[i])
		if 32 <= data[i] && data[i] <= 128 {
			chrs += string(data[i])
		} else {
			chrs += "."
		}
	}
	fmt.Println(hex + chrs)
}
