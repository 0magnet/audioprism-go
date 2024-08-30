// Package gomobile implements UI
package gomobile

import (
	"encoding/binary"
	"log"
	"sync"

	"github.com/jfreymuth/pulse"
	"golang.org/x/mobile/app"
	"golang.org/x/mobile/event/lifecycle"
	"golang.org/x/mobile/event/paint"
	"golang.org/x/mobile/event/size"
	"golang.org/x/mobile/exp/app/debug"
	"golang.org/x/mobile/exp/f32"
	"golang.org/x/mobile/exp/gl/glutil"
	"golang.org/x/mobile/gl"

	"github.com/0magnet/audioprism-go/pkg/spectrogram"
)

var (
	images                              *glutil.Images
	fps                                 *debug.FPS
	program                             gl.Program
	position                            gl.Attrib
	texCoord                            gl.Attrib
	texture                             gl.Texture
	spectrogramHistory                  [][]byte
	audioBufferLock                     sync.Mutex
	spectrogramWidth, spectrogramHeight int
	showFPS                             bool
)

// Run initializes and starts the Gomobile application
func Run(width, height, bufferSize int, fpsDisplay bool) {
	spectrogramWidth = width
	spectrogramHeight = height
	showFPS = fpsDisplay

	spectrogramHistory = make([][]byte, spectrogramWidth)
	for i := range spectrogramHistory {
		spectrogramHistory[i] = make([]byte, spectrogramHeight*4)
	}

	c, err := pulse.NewClient()
	if err != nil {
		log.Fatal(err.Error())
	}
	defer c.Close()

	stream, err := c.NewRecord(pulse.Float32Writer(func(p []float32) (int, error) {
		audioBufferLock.Lock()
		spectrogram.AudioBuffer = append(spectrogram.AudioBuffer, p...)
		if len(spectrogram.AudioBuffer) > bufferSize {
			spectrogram.AudioBuffer = spectrogram.AudioBuffer[len(spectrogram.AudioBuffer)-bufferSize:]
		}
		audioBufferLock.Unlock()
		return len(p), nil
	}))
	if err != nil {
		log.Fatal(err.Error())
	}

	stream.Start()

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

				updateSpectrogramData(glctx)
				onPaint(glctx, sz, showFPS)
				a.Publish()
				a.Send(paint.Event{})
			}
		}
	})

	stream.Stop()
}

func updateSpectrogramData(glctx gl.Context) {
	audioBufferLock.Lock()
	defer audioBufferLock.Unlock()

	chunk := spectrogram.GetAudioChunk()
	if chunk == nil {
		return
	}

	magnitudes := spectrogram.ComputeFFT(chunk)
	newColumn := make([]byte, spectrogramHeight*4)

	for y := 0; y < spectrogramHeight; y++ {
		color := spectrogram.MagnitudeToPixel(magnitudes[y])
		r, g, b, a := color.RGBA()
		newColumn[y*4+0] = byte(r >> 8)
		newColumn[y*4+1] = byte(g >> 8)
		newColumn[y*4+2] = byte(b >> 8)
		newColumn[y*4+3] = byte(a >> 8)
	}

	copy(spectrogramHistory[0:], spectrogramHistory[1:])
	spectrogramHistory[spectrogramWidth-1] = newColumn

	glctx.BindTexture(gl.TEXTURE_2D, texture)
	for i, column := range spectrogramHistory {
		glctx.TexSubImage2D(gl.TEXTURE_2D, 0, i, 0, 1, spectrogramHeight, gl.RGBA, gl.UNSIGNED_BYTE, column)
	}
}

func onStart(glctx gl.Context) {
	var err error
	program, err = glutil.CreateProgram(glctx, vertexShader, fragmentShader)
	if err != nil {
		log.Printf("error creating GL program: %v", err)
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
	glctx.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA, spectrogramWidth, spectrogramHeight, gl.RGBA, gl.UNSIGNED_BYTE, nil)
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
