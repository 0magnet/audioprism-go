package wav

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"strings"
	"testing"
)

// chunk builds a RIFF chunk, word-aligning the body the way the format
// requires.
func chunk(id string, body []byte) []byte {
	out := make([]byte, 0, 8+len(body)+1)
	out = append(out, id...)
	out = binary.LittleEndian.AppendUint32(out, uint32(len(body))) //nolint:gosec // a fixture value chosen in the test below
	out = append(out, body...)
	if len(body)%2 == 1 {
		out = append(out, 0)
	}
	return out
}

// fmtChunk builds a 16-byte fmt body.
func fmtChunk(format, channels, rate, bits int) []byte {
	b := make([]byte, 16)
	binary.LittleEndian.PutUint16(b[0:], uint16(format))   //nolint:gosec // a fixture value chosen in the test below
	binary.LittleEndian.PutUint16(b[2:], uint16(channels)) //nolint:gosec // a fixture value chosen in the test below
	binary.LittleEndian.PutUint32(b[4:], uint32(rate))     //nolint:gosec // a fixture value chosen in the test below
	blockAlign := channels * bits / 8
	binary.LittleEndian.PutUint32(b[8:], uint32(rate*blockAlign)) //nolint:gosec // byte rate; a fixture value
	binary.LittleEndian.PutUint16(b[12:], uint16(blockAlign))     //nolint:gosec // a fixture value chosen in the test below
	binary.LittleEndian.PutUint16(b[14:], uint16(bits))           //nolint:gosec // a fixture value chosen in the test below
	return b
}

// riff wraps chunks in a RIFF/WAVE container.
func riff(chunks ...[]byte) []byte {
	var body []byte
	body = append(body, "WAVE"...)
	for _, c := range chunks {
		body = append(body, c...)
	}
	out := append([]byte("RIFF"), 0, 0, 0, 0)
	binary.LittleEndian.PutUint32(out[4:8], uint32(len(body))) //nolint:gosec // a fixture value chosen in the test below
	return append(out, body...)
}

func wav16(rate, channels int, samples ...int16) []byte {
	data := make([]byte, 0, len(samples)*2)
	for _, s := range samples {
		data = binary.LittleEndian.AppendUint16(data, uint16(s)) //nolint:gosec // a fixture value chosen in the test below
	}
	return riff(chunk("fmt ", fmtChunk(formatPCM, channels, rate, 16)), chunk("data", data))
}

func read(t *testing.T, b []byte) *Audio {
	t.Helper()
	a, err := Read(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	return a
}

func closeTo(a, b float32) bool { return math.Abs(float64(a-b)) < 1e-4 }

func TestRead16BitMono(t *testing.T) {
	a := read(t, wav16(44100, 1, 0, 16384, -16384, 32767, -32768))
	if a.SampleRate != 44100 {
		t.Errorf("SampleRate = %d, want 44100", a.SampleRate)
	}
	want := []float32{0, 0.5, -0.5, 1, -1}
	if len(a.Samples) != len(want) {
		t.Fatalf("got %d samples, want %d", len(a.Samples), len(want))
	}
	for i := range want {
		if !closeTo(a.Samples[i], want[i]) {
			t.Errorf("sample %d = %v, want about %v", i, a.Samples[i], want[i])
		}
	}
}

// Samples must land in [-1, 1] whatever the input, because everything
// downstream — the spectrogram, the scope — assumes it.
func TestSamplesAreNormalized(t *testing.T) {
	a := read(t, wav16(8000, 1, 32767, -32768, 0))
	for i, s := range a.Samples {
		if s < -1.0001 || s > 1.0001 {
			t.Errorf("sample %d = %v, outside [-1, 1]", i, s)
		}
	}
}

// Stereo is mixed to mono by averaging the channels, so a signal only in the
// left channel comes back at half amplitude rather than full.
func TestStereoIsAveragedToMono(t *testing.T) {
	// Two frames: (full left, silent right), then (silent left, full right).
	a := read(t, wav16(8000, 2, 32767, 0, 0, 32767))
	if len(a.Samples) != 2 {
		t.Fatalf("got %d samples from 2 stereo frames, want 2", len(a.Samples))
	}
	for i, s := range a.Samples {
		if !closeTo(s, 0.5) {
			t.Errorf("frame %d = %v, want about 0.5 (one channel of two)", i, s)
		}
	}
}

func TestStereoInPhaseKeepsAmplitude(t *testing.T) {
	a := read(t, wav16(8000, 2, 16384, 16384))
	if len(a.Samples) != 1 || !closeTo(a.Samples[0], 0.5) {
		t.Errorf("got %v, want one sample of about 0.5", a.Samples)
	}
}

// 8-bit PCM is unsigned with 128 as silence, which is the one format where
// zero in the file is not zero in the signal.
func TestRead8BitIsUnsignedWithMidpointSilence(t *testing.T) {
	data := []byte{128, 255, 0, 192}
	b := riff(chunk("fmt ", fmtChunk(formatPCM, 1, 8000, 8)), chunk("data", data))
	a := read(t, b)
	want := []float32{0, 0.9922, -1, 0.5}
	for i := range want {
		if !closeTo(a.Samples[i], want[i]) {
			t.Errorf("sample %d = %v, want about %v", i, a.Samples[i], want[i])
		}
	}
}

// 24-bit is three bytes little-endian with the sign in the top bit, and
// sign-extending it wrongly turns quiet negative samples into loud positive
// ones.
func TestRead24BitSignExtends(t *testing.T) {
	// +1 (0x000001), -1 (0xFFFFFF), and near full scale negative (0x800000).
	data := []byte{0x01, 0x00, 0x00, 0xFF, 0xFF, 0xFF, 0x00, 0x00, 0x80}
	b := riff(chunk("fmt ", fmtChunk(formatPCM, 1, 8000, 24)), chunk("data", data))
	a := read(t, b)
	if len(a.Samples) != 3 {
		t.Fatalf("got %d samples, want 3", len(a.Samples))
	}
	if a.Samples[0] <= 0 {
		t.Errorf("0x000001 decoded as %v, want a small positive", a.Samples[0])
	}
	if a.Samples[1] >= 0 {
		t.Errorf("0xFFFFFF decoded as %v, want a small negative", a.Samples[1])
	}
	if !closeTo(a.Samples[2], -1) {
		t.Errorf("0x800000 decoded as %v, want about -1", a.Samples[2])
	}
}

func TestRead32BitPCM(t *testing.T) {
	data := make([]byte, 0, 8)
	// Via variables: a negative constant cannot be converted to uint32 directly.
	pos, neg := int32(1<<30), int32(-1)*int32(1<<30)
	data = binary.LittleEndian.AppendUint32(data, uint32(pos)) // +0.5
	data = binary.LittleEndian.AppendUint32(data, uint32(neg)) //nolint:gosec // -0.5
	b := riff(chunk("fmt ", fmtChunk(formatPCM, 1, 8000, 32)), chunk("data", data))
	a := read(t, b)
	if len(a.Samples) != 2 {
		t.Fatalf("got %d samples, want 2", len(a.Samples))
	}
	if !closeTo(a.Samples[0], 0.5) || !closeTo(a.Samples[1], -0.5) {
		t.Errorf("got %v, want about [0.5 -0.5]", a.Samples)
	}
}

func TestReadFloat32(t *testing.T) {
	data := make([]byte, 0, 12)
	for _, f := range []float32{0.25, -0.75, 1} {
		data = binary.LittleEndian.AppendUint32(data, math.Float32bits(f))
	}
	b := riff(chunk("fmt ", fmtChunk(formatFloat, 1, 48000, 32)), chunk("data", data))
	a := read(t, b)
	want := []float32{0.25, -0.75, 1}
	for i := range want {
		if !closeTo(a.Samples[i], want[i]) {
			t.Errorf("sample %d = %v, want %v", i, a.Samples[i], want[i])
		}
	}
}

// WAVE_FORMAT_EXTENSIBLE is what anything modern writes for multichannel or
// high bit depth; the real format sits in the GUID rather than the format
// field, and reading the field alone rejects the file.
func TestReadExtensibleFindsTheRealFormatInTheGUID(t *testing.T) {
	body := make([]byte, 40)
	copy(body, fmtChunk(formatExtensible, 1, 44100, 16))
	binary.LittleEndian.PutUint16(body[16:], 22) // cbSize
	binary.LittleEndian.PutUint16(body[18:], 16) // valid bits
	binary.LittleEndian.PutUint16(body[24:], formatPCM)

	data := binary.LittleEndian.AppendUint16(nil, uint16(16384))
	a := read(t, riff(chunk("fmt ", body), chunk("data", data)))
	if len(a.Samples) != 1 || !closeTo(a.Samples[0], 0.5) {
		t.Errorf("got %v, want one sample of about 0.5", a.Samples)
	}
}

// Anything that has been through an editor carries LIST or fact chunks between
// fmt and data. Assuming fixed offsets reads those as audio.
func TestReadWalksPastOtherChunks(t *testing.T) {
	data := binary.LittleEndian.AppendUint16(nil, uint16(16384))
	b := riff(
		chunk("LIST", []byte("INFOISFTsome editor")),
		chunk("fmt ", fmtChunk(formatPCM, 1, 22050, 16)),
		chunk("fact", []byte{1, 0, 0, 0}),
		chunk("data", data),
		chunk("cue ", []byte{0, 0, 0, 0}),
	)
	a := read(t, b)
	if a.SampleRate != 22050 {
		t.Errorf("SampleRate = %d, want 22050", a.SampleRate)
	}
	if len(a.Samples) != 1 || !closeTo(a.Samples[0], 0.5) {
		t.Errorf("got %v, want one sample of about 0.5", a.Samples)
	}
}

// A chunk with an odd body is padded to an even boundary. Not skipping the pad
// byte puts the reader one byte into the next chunk's id.
func TestReadHandlesOddSizedChunkPadding(t *testing.T) {
	data := binary.LittleEndian.AppendUint16(nil, uint16(16384))
	b := riff(
		chunk("LIST", []byte("odd")), // 3 bytes, needs a pad
		chunk("fmt ", fmtChunk(formatPCM, 1, 16000, 16)),
		chunk("data", data),
	)
	a := read(t, b)
	if a.SampleRate != 16000 {
		t.Errorf("SampleRate = %d, want 16000 — the pad byte was probably not skipped", a.SampleRate)
	}
}

// A recording cut short leaves a data chunk whose header promises more than
// the file holds. Refusing it loses everything that was captured.
func TestReadKeepsWhatATruncatedFileHolds(t *testing.T) {
	b := riff(chunk("fmt ", fmtChunk(formatPCM, 1, 8000, 16)),
		chunk("data", make([]byte, 8)))
	// Claim 400 bytes of data while only 8 are present.
	i := bytes.Index(b, []byte("data"))
	binary.LittleEndian.PutUint32(b[i+4:i+8], 400)

	a, err := Read(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("a truncated recording was refused: %v", err)
	}
	if len(a.Samples) != 4 {
		t.Errorf("got %d samples from the 8 bytes present, want 4", len(a.Samples))
	}
}

func TestReadRejectsWhatIsNotAWave(t *testing.T) {
	for name, b := range map[string][]byte{
		"empty":       {},
		"short":       []byte("RIFF"),
		"not riff":    append([]byte("XXXX\x00\x00\x00\x00WAVE"), 0),
		"not wave":    append([]byte("RIFF\x00\x00\x00\x00AVI "), 0),
		"text":        []byte(strings.Repeat("hello", 20)),
		"no fmt":      riff(chunk("data", []byte{0, 0})),
		"no data":     riff(chunk("fmt ", fmtChunk(formatPCM, 1, 8000, 16))),
		"short fmt":   riff(chunk("fmt ", make([]byte, 8)), chunk("data", []byte{0, 0})),
		"0 channels":  riff(chunk("fmt ", fmtChunk(formatPCM, 0, 8000, 16)), chunk("data", []byte{0, 0})),
		"0 rate":      riff(chunk("fmt ", fmtChunk(formatPCM, 1, 0, 16)), chunk("data", []byte{0, 0})),
		"odd bits":    riff(chunk("fmt ", fmtChunk(formatPCM, 1, 8000, 13)), chunk("data", []byte{0, 0})),
		"bad format":  riff(chunk("fmt ", fmtChunk(99, 1, 8000, 16)), chunk("data", []byte{0, 0})),
		"float 16bit": riff(chunk("fmt ", fmtChunk(formatFloat, 1, 8000, 16)), chunk("data", []byte{0, 0})),
	} {
		if _, err := Read(bytes.NewReader(b)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// An empty data chunk is a valid file holding no audio, not a malformed one.
func TestReadAcceptsAnEmptyDataChunk(t *testing.T) {
	b := riff(chunk("fmt ", fmtChunk(formatPCM, 1, 8000, 16)), chunk("data", nil))
	a, err := Read(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("an empty data chunk was refused: %v", err)
	}
	if len(a.Samples) != 0 {
		t.Errorf("got %d samples from no data", len(a.Samples))
	}
}

// A trailing partial frame cannot be decoded and must be dropped rather than
// read past the end of the buffer.
func TestReadDropsATrailingPartialFrame(t *testing.T) {
	// Five bytes of 16-bit stereo: one whole frame (4 bytes) and a stray byte.
	b := riff(chunk("fmt ", fmtChunk(formatPCM, 2, 8000, 16)), chunk("data", []byte{0, 0, 0, 0, 7}))
	a := read(t, b)
	if len(a.Samples) != 1 {
		t.Errorf("got %d samples, want 1 whole frame with the stray byte dropped", len(a.Samples))
	}
}

func TestReadFileReportsAMissingFile(t *testing.T) {
	if _, err := ReadFile("/nonexistent/nope.wav"); err == nil {
		t.Error("ReadFile on a missing path returned no error")
	}
}

func TestReadFileRoundTrip(t *testing.T) {
	path := t.TempDir() + "/a.wav"
	if err := writeFile(path, wav16(8000, 1, 16384, -16384)); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}
	a, err := ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if a.SampleRate != 8000 || len(a.Samples) != 2 {
		t.Errorf("got rate %d and %d samples, want 8000 and 2", a.SampleRate, len(a.Samples))
	}
}

func writeFile(path string, b []byte) error { return os.WriteFile(path, b, 0o600) }
