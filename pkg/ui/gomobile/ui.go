//go:build cgo

// Package ui pkg/ui/gomobile/ui.go
package ui

import (
	"encoding/binary"
	"log"
	"net/url"
	"sync"
	"time"

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
	"github.com/0magnet/audioprism-go/pkg/wsaudio/wscodec"
)

var (
	images         *glutil.Images
	fps            *debug.FPS
	program        gl.Program
	position       gl.Attrib
	texCoord       gl.Attrib
	texture        gl.Texture
	sgHist         [][]byte
	sgHistIndex    int
	width, height  int
	bufferWidth    int
	bufferHeight   int
	showFPS        bool
	sampleBuffer   []float32
	overlapSamples []float32
	audioChan      chan []float32
	historyMutex   sync.RWMutex

	// Persistent GL buffers
	vertexBuffer   gl.Buffer
	texCoordBuf    gl.Buffer
	buffersCreated bool

	// Reusable CPU-side texture buffer
	tempBuffer []byte

	// Column queue for smooth scrolling (same approach as WASM)
	columnQueue      [][]byte
	columnQueueMutex sync.Mutex

	// Time-based column advancement
	lastRenderTime time.Time
	columnsPerSec  float64
	columnFraction float64

	// Track if texture needs resize
	texWidth, texHeight int
)

// Run initializes and starts the Gomobile application
func Run(wid, hei, fpsRate, bSize int, fpsDisp bool, wsURL string) { //nolint:revive
	width = wid
	height = hei
	showFPS = fpsDisp

	bufferWidth = 2048
	bufferHeight = 1024

	sgHist = make([][]byte, bufferWidth)
	for i := range sgHist {
		sgHist[i] = make([]byte, bufferHeight*4)
	}

	columnsPerSec = float64(sg.SampleRate) / float64(sg.S.StepSize())

	tempBuffer = make([]byte, width*height*4)

	audioChan = make(chan []float32, 100)

	go processAudioThread()
	var stream *pulse.RecordStream
	if wsURL == "" {
		audioCtx, err := pulse.NewClient()
		if err != nil {
			log.Fatal(err)
		}
		defer audioCtx.Close()
		stream, err = audioCtx.NewRecord(pulse.Float32Writer(processAudio), pulse.RecordSampleRate(sg.SampleRate), pulse.RecordLatency(0.1))
		if err != nil {
			log.Fatal(err)
		}
		stream.Start()
		defer stream.Stop()
	} else {
		u, err := url.Parse(wsURL)
		if err != nil {
			log.Fatal("Invalid WebSocket URL:", err)
		}
		origin := u.Scheme + "://" + u.Host
		ws, err := websocket.Dial(wsURL, "", origin)
		if err != nil {
			log.Fatal("WebSocket connection failed:", err)
		}
		defer func() {
			if err := ws.Close(); err != nil {
				log.Println("Error closing WebSocket:", err)
			}
		}()
		go func() {
			for {
				// wscodec.Samples takes either encoding: a binary frame
				// from a current server, base64 in a text frame from one
				// that has not been upgraded.
				var floatData []float32
				err := wscodec.Samples.Receive(ws, &floatData)
				if err != nil {
					log.Println("Error receiving WebSocket data:", err)
					continue
				}
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
				// Update rendering dimensions to match actual window
				newW := sz.WidthPx
				newH := sz.HeightPx
				if newW > 0 && newH > 0 && (newW != width || newH != height) {
					width = newW
					height = newH
					tempBuffer = make([]byte, width*height*4)
					if glctx != nil && (texWidth != width || texHeight != height) {
						glctx.BindTexture(gl.TEXTURE_2D, texture)
						glctx.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA, width, height, gl.RGBA, gl.UNSIGNED_BYTE, nil)
						texWidth = width
						texHeight = height
					}
				}
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

// Audio callback - just send to channel
func processAudio(p []float32) (int, error) {
	samples := make([]float32, len(p))
	copy(samples, p)

	select {
	case audioChan <- samples:
	default:
	}

	return len(p), nil
}

// Processing goroutine - pushes columns to queue (not directly to sgHist)
func processAudioThread() {
	fftSize := sg.S.GetDFTSize()
	overlapSamples = make([]float32, fftSize)

	for samples := range audioChan {
		sampleBuffer = append(sampleBuffer, samples...)

		newSize := sg.S.GetDFTSize()
		if newSize != fftSize {
			fftSize = newSize
			overlapSamples = make([]float32, fftSize)
			sampleBuffer = nil
			continue
		}

		stepSize := sg.S.StepSize()
		for len(sampleBuffer) >= stepSize {
			copy(overlapSamples, overlapSamples[stepSize:])
			copy(overlapSamples[stepSize:], sampleBuffer[:fftSize-stepSize])
			sampleBuffer = sampleBuffer[stepSize:]

			magnitudes := sg.ComputeFFT(overlapSamples)

			newColumn := make([]byte, bufferHeight*4)
			for y := 0; y < bufferHeight; y++ {
				freq := float64(y) / float64(bufferHeight) * 12000
				bin := int(freq * float64(fftSize) / float64(sg.SampleRate))
				if bin < len(magnitudes) {
					color := sg.MagnitudeToPixel(magnitudes[bin])
					r, g, b, a := color.RGBA()
					newColumn[y*4+0] = byte(r >> 8 & 0xFF)
					newColumn[y*4+1] = byte(g >> 8 & 0xFF)
					newColumn[y*4+2] = byte(b >> 8 & 0xFF)
					newColumn[y*4+3] = byte(a >> 8 & 0xFF)
				} else {
					newColumn[y*4+0] = 0
					newColumn[y*4+1] = 0
					newColumn[y*4+2] = 0
					newColumn[y*4+3] = 255
				}
			}

			columnQueueMutex.Lock()
			columnQueue = append(columnQueue, newColumn)
			if len(columnQueue) > 200 {
				columnQueue = columnQueue[len(columnQueue)-100:]
			}
			columnQueueMutex.Unlock()
		}
	}
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
	texture = glctx.CreateTexture()
	glctx.BindTexture(gl.TEXTURE_2D, texture)
	glctx.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
	glctx.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
	glctx.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	glctx.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	glctx.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA, width, height, gl.RGBA, gl.UNSIGNED_BYTE, nil)
	texWidth = width
	texHeight = height

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

	vertexBuffer = glctx.CreateBuffer()
	glctx.BindBuffer(gl.ARRAY_BUFFER, vertexBuffer)
	glctx.BufferData(gl.ARRAY_BUFFER, f32.Bytes(binary.LittleEndian, quadVertexData...), gl.STATIC_DRAW)

	texCoordBuf = glctx.CreateBuffer()
	glctx.BindBuffer(gl.ARRAY_BUFFER, texCoordBuf)
	glctx.BufferData(gl.ARRAY_BUFFER, f32.Bytes(binary.LittleEndian, quadTexCoordData...), gl.STATIC_DRAW)

	buffersCreated = true
	lastRenderTime = time.Now()
}

func onStop(glctx gl.Context) {
	glctx.DeleteProgram(program)
	glctx.DeleteTexture(texture)
	if buffersCreated {
		glctx.DeleteBuffer(vertexBuffer)
		glctx.DeleteBuffer(texCoordBuf)
		buffersCreated = false
	}
	if showFPS {
		fps.Release()
	}
	images.Release()
}

func onPaint(glctx gl.Context, sz size.Event, showFPS bool) {
	// Time-based column advancement with batch mutex ops
	now := time.Now()
	elapsed := now.Sub(lastRenderTime).Seconds()
	lastRenderTime = now

	columnFraction += elapsed * columnsPerSec
	columnsToAdvance := int(columnFraction)
	columnFraction -= float64(columnsToAdvance)

	columnQueueMutex.Lock()
	drain := columnsToAdvance
	if drain > len(columnQueue) {
		drain = len(columnQueue)
	}
	// Keep queue at most 2 deep to minimize latency
	if len(columnQueue)-drain > 2 {
		drain = len(columnQueue) - 1
	}
	// Batch copy to reduce lock contention
	drained := make([][]byte, drain)
	copy(drained, columnQueue[:drain])
	columnQueue = columnQueue[drain:]
	columnQueueMutex.Unlock()

	if len(drained) > 0 {
		historyMutex.Lock()
		for _, col := range drained {
			sgHist[sgHistIndex] = col
			sgHistIndex = (sgHistIndex + 1) % bufferWidth
		}
		historyMutex.Unlock()
	}

	glctx.ClearColor(0, 0, 0, 1)
	glctx.Clear(gl.COLOR_BUFFER_BIT)

	glctx.UseProgram(program)

	glctx.BindBuffer(gl.ARRAY_BUFFER, vertexBuffer)
	glctx.EnableVertexAttribArray(position)
	glctx.VertexAttribPointer(position, 3, gl.FLOAT, false, 0, 0)

	glctx.BindBuffer(gl.ARRAY_BUFFER, texCoordBuf)
	glctx.EnableVertexAttribArray(texCoord)
	glctx.VertexAttribPointer(texCoord, 2, gl.FLOAT, false, 0, 0)

	glctx.BindTexture(gl.TEXTURE_2D, texture)

	localW := width
	localH := height

	// Clear temp buffer
	for i := range tempBuffer {
		tempBuffer[i] = 0
	}

	historyMutex.RLock()
	localIndex := sgHistIndex
	startIndex := (localIndex - localW + bufferWidth) % bufferWidth
	for x := 0; x < localW; x++ {
		index := (startIndex + x) % bufferWidth
		for y := 0; y < localH; y++ {
			bufferY := int(float64(y) / float64(localH) * float64(bufferHeight))
			if bufferY < bufferHeight {
				offset := (y*localW + x) * 4
				bufferOffset := bufferY * 4
				tempBuffer[offset+0] = sgHist[index][bufferOffset+0]
				tempBuffer[offset+1] = sgHist[index][bufferOffset+1]
				tempBuffer[offset+2] = sgHist[index][bufferOffset+2]
				tempBuffer[offset+3] = sgHist[index][bufferOffset+3]
			}
		}
	}
	historyMutex.RUnlock()

	glctx.TexSubImage2D(gl.TEXTURE_2D, 0, 0, 0, localW, localH, gl.RGBA, gl.UNSIGNED_BYTE, tempBuffer)
	glctx.DrawArrays(gl.TRIANGLE_STRIP, 0, 4)

	glctx.DisableVertexAttribArray(position)
	glctx.DisableVertexAttribArray(texCoord)

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
