// Package wscodec carries the wsaudio wire format over a WebSocket for
// the Go ends of it — the servers and the tcell, fyne and gomobile
// clients. The browser client does its own decoding through syscall/js.
//
// It is a package of its own rather than a file in wsaudio because the
// browser client imports wsaudio, and importing golang.org/x/net/websocket
// there drags net/http and crypto/tls into the wasm binary: measured,
// 3.1 MB becomes 5.2 MB. The split costs one import line in four files.
package wscodec

import (
	"encoding/base64"

	"golang.org/x/net/websocket"

	"github.com/0magnet/audioprism-go/pkg/wsaudio"
)

// Samples is the codec both ends of a Go-to-Go audio stream use.
//
// It is a codec rather than plain websocket.Message calls because only a
// codec is handed the frame's payload type, and that byte is the entire
// version negotiation: a text frame is the old base64 encoding, a binary
// frame the raw samples. x/net/websocket's own Message codec throws the
// payload type away and will hand a binary frame to a *string, so the
// receiver could not otherwise tell the two apart without guessing.
//
// Send takes []float32 (encoded here) or a []byte already in the wire
// layout — the file streamer reads exactly those bytes from ffmpeg and
// has nothing to convert. Receive takes *[]float32.
var Samples = websocket.Codec{Marshal: marshalSamples, Unmarshal: unmarshalSamples}

func marshalSamples(v interface{}) ([]byte, byte, error) {
	switch s := v.(type) {
	case []float32:
		return wsaudio.Float32ToBytes(s), websocket.BinaryFrame, nil
	case []byte:
		return s, websocket.BinaryFrame, nil
	}
	return nil, websocket.UnknownFrame, websocket.ErrNotSupported
}

func unmarshalSamples(data []byte, payloadType byte, v interface{}) error {
	dst, ok := v.(*[]float32)
	if !ok {
		return websocket.ErrNotSupported
	}
	if payloadType == websocket.TextFrame {
		b, err := base64.StdEncoding.DecodeString(string(data))
		if err != nil {
			return err
		}
		*dst = wsaudio.BytesToFloat32(b)
		return nil
	}
	*dst = wsaudio.BytesToFloat32(data)
	return nil
}
