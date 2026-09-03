// Package wav reads a RIFF/WAVE file into the mono float32 stream the
// spectrogram works in.
//
// It exists because the original audioprism has a second mode besides the live
// one — give it a WAV and an output path and it writes the spectrogram to an
// image — and that mode is what makes the thing checkable. A live capture can
// only ever be compared by eye against another live capture of something else.
// A file renders the same picture every time, on any machine, with no audio
// device present, so two implementations can be held against each other and the
// difference measured rather than argued about.
//
// Only what audioprism itself accepts is supported: uncompressed PCM, 8/16/24/32
// bit integer or 32/64 bit float, any channel count (mixed down to mono, as the
// original's audio path is mono).
package wav

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
)

// Audio is a decoded WAV: mono samples in [-1, 1] and the rate they were
// recorded at.
type Audio struct {
	Samples    []float32
	SampleRate int
}

const (
	formatPCM        = 1
	formatFloat      = 3
	formatExtensible = 0xFFFE
)

// ReadFile decodes the WAV at path.
func ReadFile(path string) (*Audio, error) {
	f, err := os.Open(path) //nolint:gosec // the path is the WAV file the caller asked to read
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck
	return Read(f)
}

// Read decodes a WAV from r.
func Read(r io.Reader) (*Audio, error) {
	buf, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if len(buf) < 12 || string(buf[0:4]) != "RIFF" || string(buf[8:12]) != "WAVE" {
		return nil, fmt.Errorf("not a RIFF/WAVE file")
	}

	var (
		format, channels, bits int
		rate                   int
		data                   []byte
		haveFmt                bool
	)

	// Walk the chunk list rather than assuming fmt then data at fixed offsets:
	// anything that has been through an editor tends to carry LIST, fact or
	// cue chunks in between, and a fixed offset reads those as audio.
	for off := 12; off+8 <= len(buf); {
		id := string(buf[off : off+4])
		size := int(binary.LittleEndian.Uint32(buf[off+4 : off+8]))
		body := off + 8
		if size < 0 || body+size > len(buf) {
			// A truncated final chunk is common in recordings cut short; take
			// what is there rather than refusing the file.
			size = len(buf) - body
			if size < 0 {
				break
			}
		}
		switch id {
		case "fmt ":
			if size < 16 {
				return nil, fmt.Errorf("fmt chunk is %d bytes, need at least 16", size)
			}
			format = int(binary.LittleEndian.Uint16(buf[body : body+2]))
			channels = int(binary.LittleEndian.Uint16(buf[body+2 : body+4]))
			rate = int(binary.LittleEndian.Uint32(buf[body+4 : body+8]))
			bits = int(binary.LittleEndian.Uint16(buf[body+14 : body+16]))
			if format == formatExtensible && size >= 40 {
				// The real format sits in the GUID's first two bytes.
				format = int(binary.LittleEndian.Uint16(buf[body+24 : body+26]))
			}
			haveFmt = true
		case "data":
			data = buf[body : body+size]
		}
		off = body + size
		if size%2 == 1 {
			off++ // chunks are word-aligned
		}
	}

	if !haveFmt {
		return nil, fmt.Errorf("no fmt chunk")
	}
	if data == nil {
		return nil, fmt.Errorf("no data chunk")
	}
	if channels < 1 {
		return nil, fmt.Errorf("%d channels", channels)
	}
	if rate < 1 {
		return nil, fmt.Errorf("sample rate %d", rate)
	}

	decode, frameBytes, err := decoderFor(format, bits)
	if err != nil {
		return nil, err
	}

	stride := frameBytes * channels
	n := len(data) / stride
	out := make([]float32, n)
	for i := 0; i < n; i++ {
		// Mixed down to mono by averaging, which is what a mono capture of the
		// same material would have given.
		sum := 0.0
		base := i * stride
		for c := 0; c < channels; c++ {
			sum += decode(data[base+c*frameBytes:])
		}
		out[i] = float32(sum / float64(channels))
	}
	return &Audio{Samples: out, SampleRate: rate}, nil
}

// decoderFor returns a function reading one sample from the front of a slice,
// normalized to [-1, 1], and how many bytes that sample occupies.
func decoderFor(format, bits int) (func([]byte) float64, int, error) {
	switch {
	case format == formatPCM && bits == 8:
		// 8-bit PCM is unsigned with 128 as silence; every other width is signed.
		return func(b []byte) float64 { return (float64(b[0]) - 128) / 128 }, 1, nil
	case format == formatPCM && bits == 16:
		return func(b []byte) float64 {
			return float64(int16(binary.LittleEndian.Uint16(b))) / 32768 //nolint:gosec // PCM samples are signed; this reinterprets the bits, it does not convert a value
		}, 2, nil
	case format == formatPCM && bits == 24:
		return func(b []byte) float64 {
			v := int32(b[0]) | int32(b[1])<<8 | int32(b[2])<<16
			if v&0x800000 != 0 {
				v |= ^0xFFFFFF // sign-extend the 24th bit
			}
			return float64(v) / 8388608
		}, 3, nil
	case format == formatPCM && bits == 32:
		return func(b []byte) float64 {
			return float64(int32(binary.LittleEndian.Uint32(b))) / 2147483648 //nolint:gosec // as above
		}, 4, nil
	case format == formatFloat && bits == 32:
		return func(b []byte) float64 {
			return float64(math.Float32frombits(binary.LittleEndian.Uint32(b)))
		}, 4, nil
	case format == formatFloat && bits == 64:
		return func(b []byte) float64 {
			return math.Float64frombits(binary.LittleEndian.Uint64(b))
		}, 8, nil
	}
	return nil, 0, fmt.Errorf("unsupported WAV format %d at %d bits (want uncompressed PCM or float)", format, bits)
}
