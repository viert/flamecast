package mpeg

import (
	"errors"
)

type (
	FrameHeader []byte

	FrameMPEGVersion byte
	FrameLayer       byte
	FrameEmphasis    byte
	FrameChannelMode byte
	FrameSampleRate  int
	FrameBitRate     int
)

// FrameMPEGVersion value mappings as documented at https://www.mp3-tech.org/programmer/frame_header.html
const (
	VersionMPEG2_5 FrameMPEGVersion = iota
	VersionMPEGReserved
	VersionMPEG2
	VersionMPEG1
)

// FrameLayer value mappings as documented at https://www.mp3-tech.org/programmer/frame_header.html
const (
	LayerReserved FrameLayer = iota
	Layer3
	Layer2
	Layer1
)

// FrameEmphasis value mappings as documented at https://www.mp3-tech.org/programmer/frame_header.html
const (
	EmphasisNone FrameEmphasis = iota
	Emphasis5015ms
	EmphasisReserved
	EmphasisCCITJ17
)

// FrameChannelMode value mappings as documented at https://www.mp3-tech.org/programmer/frame_header.html
const (
	ChannelModeStereo FrameChannelMode = iota
	ChannelModeJointStereo
	ChannelModeDualChannel
	ChannelModeSingleChannel
)

// SampleRateInvalid is a special value of FrameSampleRate indicating that
// determination of sample rate is not possible
const SampleRateInvalid FrameSampleRate = 0

// BitRateInvalid is a special unallowed value of FrameBitRate
const BitRateInvalid FrameBitRate = -1

var (
	bitRates = [4][4][16]FrameBitRate{
		// MPEG Version 2.5
		{
			{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, BitRateInvalid},
			{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160, BitRateInvalid},
			{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160, BitRateInvalid},
			{0, 32, 48, 56, 64, 80, 96, 112, 128, 144, 160, 176, 192, 224, 256, BitRateInvalid},
		},
		// MPEG Version Reserved
		{
			{
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
			},
			{
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
			},
			{
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
			},
			{
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
				BitRateInvalid,
			},
		},
		// MPEG Version 2
		{
			{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, BitRateInvalid},
			{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160, BitRateInvalid},
			{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160, BitRateInvalid},
			{0, 32, 48, 56, 64, 80, 96, 112, 128, 144, 160, 176, 192, 224, 256, BitRateInvalid},
		},
		// MPEG Version 1
		{
			{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, BitRateInvalid},
			{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, BitRateInvalid},
			{0, 32, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 384, BitRateInvalid},
			{0, 32, 64, 96, 128, 160, 192, 224, 256, 288, 320, 352, 384, 416, 448, BitRateInvalid},
		},
	}

	sampleRates = [4][4]FrameSampleRate{
		// MPEG Version 2.5
		{
			11025,
			12000,
			8000,
			SampleRateInvalid,
		},
		// MPEG Version sReserved
		{
			SampleRateInvalid,
			SampleRateInvalid,
			SampleRateInvalid,
			SampleRateInvalid,
		},
		// MPEG Version 2
		{
			22050,
			24000,
			16000,
			SampleRateInvalid,
		},
		// MPEG Version 1
		{
			44100,
			48000,
			32000,
			SampleRateInvalid,
		},
	}

	// https://hydrogenaud.io/index.php/topic,85125.0.html
	samplesPerFrame = [4][4]int{
		//  3    2     1               Version
		{0, 576, 1152, 384},  //       2.5
		{0, 0, 0, 0},         //       Reserved
		{0, 576, 1152, 384},  //       2
		{0, 1152, 1152, 384}, //       1
	}

	slotSizes = [4]int{0, 1, 1, 4}
)

// Layer returns MPEG layer of the frame
func (fh FrameHeader) Layer() FrameLayer {
	return FrameLayer((fh[1] >> 1) & 0x03)
}

// Version returns MPEG version of the frame
func (fh FrameHeader) Version() FrameMPEGVersion {
	return FrameMPEGVersion((fh[1] >> 3) & 0x03)
}

// Protected returns true if the frame is CRC protected
func (fh FrameHeader) Protected() bool {
	return (fh[1] & 0x01) != 0x01
}

// Emphasis returns the emphasis indication of the frame whatever it means
// MPEG documentation says:
// "The emphasis indication is here to tell the decoder that the file must be de-emphasized,
// ie the decoder must 're-equalize' the sound after a Dolby-like noise supression. It is rarely used."
// See https://www.mp3-tech.org/programmer/frame_header.html
func (fh FrameHeader) Emphasis() FrameEmphasis {
	return FrameEmphasis((fh[3] & 0x03))
}

// ChannelMode returns the channel mode configuration
func (fh FrameHeader) ChannelMode() FrameChannelMode {
	return FrameChannelMode((fh[3] >> 6) & 0x03)
}

// SampleRate calculates the samplerate of the frame
func (fh FrameHeader) SampleRate() FrameSampleRate {
	freqIndex := (fh[2] >> 2) & 0x03
	return FrameSampleRate(sampleRates[fh.Version()][freqIndex])
}

// BitRate returns the calculated bit rate from the header
func (fh FrameHeader) BitRate() FrameBitRate {
	brIndex := (fh[2] >> 4) & 0x0F
	return FrameBitRate(bitRates[fh.Version()][fh.Layer()][brIndex] * 1000)
}

// Pad returns true if there are padding bits to match the actual bitrate
func (fh FrameHeader) Pad() bool {
	return ((fh[2] >> 1) & 0x01) == 0x01
}

// SideInfoLength returns the side info length
// as it's implemented in github.com:tcolgate/mp3
// It's not actually used in any calculations though
func (fh FrameHeader) SideInfoLength() (int, error) {
	switch fh.Version() {
	case VersionMPEG1:
		switch fh.ChannelMode() {
		case ChannelModeSingleChannel:
			return 17, nil
		case ChannelModeStereo, ChannelModeJointStereo, ChannelModeDualChannel:
			return 32, nil
		default:
			return 0, errors.New("bad channel mode")
		}
	case VersionMPEG2, VersionMPEG2_5:
		switch fh.ChannelMode() {
		case ChannelModeSingleChannel:
			return 9, nil
		case ChannelModeStereo, ChannelModeJointStereo, ChannelModeDualChannel:
			return 17, nil
		default:
			return 0, errors.New("bad channel mode")
		}
	default:
		return 0, errors.New("bad version")
	}
}

// NumSamples returns number of samples in frame
func (fh FrameHeader) NumSamples() int {
	return samplesPerFrame[fh.Version()][fh.Layer()]
}

// FrameSize calculates the frame size
func (fh FrameHeader) FrameSize() int {
	bps := float64(fh.NumSamples()) / 8
	size := (bps * float64(fh.BitRate())) / float64(fh.SampleRate())
	if fh.Pad() {
		size += float64(slotSizes[fh.Layer()])
	}
	return int(size)

}

func FrameHeaderValid(data []byte) bool {
	if len(data) < 4 {
		return false
	}
	if data[0] == 0xFF && data[1]&0xE0 == 0xE0 {
		hdr := FrameHeader(data[:4])
		if hdr.Emphasis() != EmphasisReserved &&
			hdr.Layer() != LayerReserved &&
			hdr.Version() != VersionMPEGReserved &&
			hdr.SampleRate() != SampleRateInvalid &&
			hdr.BitRate() != BitRateInvalid {
			return true
		}
	}
	return false
}
