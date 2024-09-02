// Package gomobile pkg/gomobile/ui.go
package gomobile

import (
	"encoding/base64"
	"encoding/binary"
	"log"
	"math"
	"net/url"

	"github.com/jfreymuth/pulse"
	"golang.org/x/mobile/app"
	"golang.org/x/mobile/event/lifecycle"
	"golang.org/x/mobile/event/paint"
	"golang.org/x/mobile/event/size"
	"golang.org/x/mobile/exp/app/debug"
	"golang.org/x/mobile/exp/f32"
	"golang.org/x/mobile/exp/gl/glutil"
	"golang.org/x/mobile/gl"
	"golang.org/x/net/websocket"

	sg "github.com/0magnet/audioprism-go/pkg/spectrogram"
)

var (
	images        *glutil.Images
	fps           *debug.FPS
	program       gl.Program
	position      gl.Attrib
	texCoord      gl.Attrib
	texture       gl.Texture
	sgHist        [][]byte
	sgHistIndex   int
	width, height int
	showFPS       bool
)

// Run initializes and starts the Gomobile application
func Run(w, h, _, _ int, fpsDisp bool, wsURL string) {
	width = w
	height = h
	showFPS = fpsDisp

	// Initialize the spectrogram history buffer
	sgHist = make([][]byte, width)
	for i := range sgHist {
		sgHist[i] = make([]byte, height*4) // RGBA
	}
	var stream *pulse.RecordStream
	if wsURL == "" {
		// Initialize PulseAudio client and stream
		audioCtx, err := pulse.NewClient()
		if err != nil {
			log.Fatal(err)
		}
		defer audioCtx.Close()

		stream, err = audioCtx.NewRecord(pulse.Float32Writer(processAudio), pulse.RecordLatency(0.1))
		if err != nil {
			log.Fatal(err)
		}
		stream.Start()
		defer stream.Stop()
	} else {
		// Parse the WebSocket URL to determine the correct origin
		u, err := url.Parse(wsURL)
		if err != nil {
			log.Fatal("Invalid WebSocket URL:", err)
		}
		origin := u.Scheme + "://" + u.Host

		// Connect to WebSocket server using the provided URL and dynamic origin
		ws, err := websocket.Dial(wsURL, "", origin)
		if err != nil {
			log.Fatal("WebSocket connection failed:", err)
		}
		defer func() {
			if err := ws.Close(); err != nil {
				log.Println("Error closing WebSocket:", err)
			}
		}()

		// Start listening to WebSocket in a separate goroutine
		go func() {
			for {
				var encodedData string
				err := websocket.Message.Receive(ws, &encodedData)
				if err != nil {
					log.Println("Error receiving WebSocket data:", err)
					continue
				}

				// Decode Base64 data back to []byte
				data, err := base64.StdEncoding.DecodeString(encodedData)
				if err != nil {
					log.Println("Error decoding Base64 data:", err)
					continue
				}

				// Convert received []byte data back to []float32
				floatData := make([]float32, len(data)/4) // Each float32 is 4 bytes
				for i := range floatData {
					floatData[i] = math.Float32frombits(uint32(data[i*4]) |
						uint32(data[i*4+1])<<8 |
						uint32(data[i*4+2])<<16 |
						uint32(data[i*4+3])<<24)
				}

				// Process the audio data directly
				_, err = processAudio(floatData)
				if err != nil {
					log.Fatal(err)
				}
			}
		}()

	}

	app.Main(func(a app.App) {
		var glctx gl.Context
		var sz size.Event
		for e := range a.Events() {
			switch e := a.Filter(e).(type) {
			case lifecycle.Event:
				switch e.Crosses(lifecycle.StageVisible) {
				case lifecycle.CrossOn:
					glctx, _ = e.DrawContext.(gl.Context)
					onStart(glctx)
					a.Send(paint.Event{})
				case lifecycle.CrossOff:
					onStop(glctx)
					glctx = nil
				}
			case size.Event:
				sz = e
			case paint.Event:
				if glctx == nil || e.External {
					continue
				}
				onPaint(glctx, sz, showFPS)
				a.Publish()
				a.Send(paint.Event{})
			}
		}
	})
}

func processAudio(p []float32) (int, error) {
	start := 0
	step := sg.FFTSize / 2
	for len(p)-start >= sg.FFTSize {
		magnitudes := sg.ComputeFFT(p[start : start+sg.FFTSize])
		start += step

		newColumn := make([]byte, height*4)
		for y := 0; y < height; y++ {
			freq := float64(y) / float64(height) * 12000
			bin := int(freq * float64(sg.FFTSize) / 44100)
			if bin < len(magnitudes) {
				color := sg.MagnitudeToPixel(magnitudes[bin])
				r, g, b, a := color.RGBA()
				newColumn[y*4+0] = byte(r >> 8)
				newColumn[y*4+1] = byte(g >> 8)
				newColumn[y*4+2] = byte(b >> 8)
				newColumn[y*4+3] = byte(a >> 8)
			} else {
				newColumn[y*4+0] = 0
				newColumn[y*4+1] = 0
				newColumn[y*4+2] = 0
				newColumn[y*4+3] = 255
			}
		}

		sgHist[sgHistIndex] = newColumn
		sgHistIndex = (sgHistIndex + 1) % width
	}
	return len(p), nil
}

func onStart(glctx gl.Context) {
	var err error
	program, err = glutil.CreateProgram(glctx, vertexShader, fragmentShader)
	if err != nil {
		log.Fatalf("error creating GL program: %v", err)
		return
	}

	position = glctx.GetAttribLocation(program, "position")
	texCoord = glctx.GetAttribLocation(program, "texCoord")

	images = glutil.NewImages(glctx)
	if showFPS {
		fps = debug.NewFPS(images)
	}

	// Initialize texture
	texture = glctx.CreateTexture()
	glctx.BindTexture(gl.TEXTURE_2D, texture)
	glctx.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
	glctx.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
	glctx.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	glctx.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	glctx.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA, width, height, gl.RGBA, gl.UNSIGNED_BYTE, nil)
}

func onStop(glctx gl.Context) {
	glctx.DeleteProgram(program)
	glctx.DeleteTexture(texture)
	if showFPS {
		fps.Release()
	}
	images.Release()
}

func onPaint(glctx gl.Context, sz size.Event, showFPS bool) {
	glctx.ClearColor(0, 0, 0, 1)
	glctx.Clear(gl.COLOR_BUFFER_BIT)

	glctx.UseProgram(program)

	quadVertexData := []float32{
		-1, -1, 0,
		1, -1, 0,
		-1, 1, 0,
		1, 1, 0,
	}
	quadTexCoordData := []float32{
		0, 0,
		1, 0,
		0, 1,
		1, 1,
	}

	vertexBuffer := glctx.CreateBuffer()
	glctx.BindBuffer(gl.ARRAY_BUFFER, vertexBuffer)
	glctx.BufferData(gl.ARRAY_BUFFER, f32.Bytes(binary.LittleEndian, quadVertexData...), gl.STATIC_DRAW)

	glctx.EnableVertexAttribArray(position)
	glctx.VertexAttribPointer(position, 3, gl.FLOAT, false, 0, 0)

	texCoordBuffer := glctx.CreateBuffer()
	glctx.BindBuffer(gl.ARRAY_BUFFER, texCoordBuffer)
	glctx.BufferData(gl.ARRAY_BUFFER, f32.Bytes(binary.LittleEndian, quadTexCoordData...), gl.STATIC_DRAW)

	glctx.EnableVertexAttribArray(texCoord)
	glctx.VertexAttribPointer(texCoord, 2, gl.FLOAT, false, 0, 0)

	glctx.BindTexture(gl.TEXTURE_2D, texture)
	for x := 0; x < len(sgHist); x++ {
		index := (sgHistIndex + x) % len(sgHist)
		glctx.TexSubImage2D(gl.TEXTURE_2D, 0, x, 0, 1, len(sgHist[0])/4, gl.RGBA, gl.UNSIGNED_BYTE, sgHist[index])
	}
	glctx.DrawArrays(gl.TRIANGLE_STRIP, 0, 4)

	glctx.DisableVertexAttribArray(position)
	glctx.DisableVertexAttribArray(texCoord)
	glctx.DeleteBuffer(vertexBuffer)
	glctx.DeleteBuffer(texCoordBuffer)

	if showFPS {
		fps.Draw(sz)
	}
}

const vertexShader = `
#version 100
attribute vec4 position;
attribute vec2 texCoord;
varying vec2 v_texCoord;
void main() {
	gl_Position = position;
	v_texCoord = texCoord;
}`

const fragmentShader = `
#version 100
precision mediump float;
varying vec2 v_texCoord;
uniform sampler2D texture;
void main() {
	gl_FragColor = texture2D(texture, v_texCoord);
}`
