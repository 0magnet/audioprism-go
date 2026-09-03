package wscodec

import (
	"encoding/base64"
	"math"
	"testing"

	"golang.org/x/net/websocket"

	"github.com/0magnet/audioprism-go/pkg/wsaudio"
)

// The compatibility claim, checked without a socket: whichever frame type
// arrives, the samples come out the same. A binary frame is the raw
// layout, a text frame the base64 an un-upgraded server still sends.
func TestSamplesCodecAcceptsBothFrameTypes(t *testing.T) {
	want := []float32{0.25, -0.5, 0, float32(math.Inf(-1))}
	raw := wsaudio.Float32ToBytes(want)

	cases := []struct {
		name        string
		payload     []byte
		payloadType byte
	}{
		{"binary frame", raw, websocket.BinaryFrame},
		{"legacy text frame", []byte(base64.StdEncoding.EncodeToString(raw)), websocket.TextFrame},
	}
	for _, c := range cases {
		var got []float32
		if err := unmarshalSamples(c.payload, c.payloadType, &got); err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if len(got) != len(want) {
			t.Errorf("%s: decoded %d samples, want %d", c.name, len(got), len(want))
			continue
		}
		for i := range want {
			if math.Float32bits(got[i]) != math.Float32bits(want[i]) {
				t.Errorf("%s: sample %d = %v, want %v", c.name, i, got[i], want[i])
			}
		}
	}
}

func TestSamplesCodecMarshalsBinary(t *testing.T) {
	in := []float32{1, -2}
	data, payloadType, err := marshalSamples(in)
	if err != nil {
		t.Fatalf("marshal []float32: %v", err)
	}
	if payloadType != websocket.BinaryFrame {
		t.Errorf("payload type %d, want BinaryFrame (%d)", payloadType, websocket.BinaryFrame)
	}
	if string(data) != string(wsaudio.Float32ToBytes(in)) {
		t.Errorf("marshaled % x, want % x", data, wsaudio.Float32ToBytes(in))
	}

	// Bytes already in the wire layout (the file streamer's ffmpeg output)
	// go out untouched, in a binary frame all the same.
	data, payloadType, err = marshalSamples([]byte{1, 2, 3, 4})
	if err != nil || payloadType != websocket.BinaryFrame || len(data) != 4 {
		t.Errorf("marshal []byte = (% x, %d, %v), want the same 4 bytes in a binary frame", data, payloadType, err)
	}

	if _, _, err := marshalSamples("a string"); err == nil {
		t.Error("marshaling a string was accepted; only samples and raw bytes should be")
	}
	var wrong []int
	if err := unmarshalSamples(nil, websocket.BinaryFrame, &wrong); err == nil {
		t.Error("unmarshaling into *[]int was accepted; only *[]float32 should be")
	}
}

func TestUnmarshalRejectsBadBase64(t *testing.T) {
	var got []float32
	if err := unmarshalSamples([]byte("!!!not base64!!!"), websocket.TextFrame, &got); err == nil {
		t.Error("a text frame that is not base64 decoded without error")
	}
}
