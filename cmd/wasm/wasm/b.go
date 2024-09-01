//go:build js && wasm

// Package main cmd/wasm/wasm/b.go
package main

import (
	"encoding/base64"
	"log"
	"math"
	"syscall/js"

	sg "github.com/0magnet/audioprism-go/pkg/spectrogram"
	"github.com/0magnet/audioprism-go/pkg/wgl"
)

var (
	w, h                                       int
	gl, t, wskt, sP, uSampler, vPos, vTexCoord js.Value //nolint:unused
	sHist                                      [][]byte
	sHistIndex                                 int
	rndrFr                                     js.Func
	glTypes                                    wgl.GLTypes
	historySize                                = 600
)

func main() {
	initWS()
	initGL()
	initShaders()
	spectexture()
	initHist()
	renderLoop()
	select {}
}

func initWS() {
	protocol := "ws"
	if js.Global().Get("window").Get("location").Get("protocol").String() == "https:" {
		protocol = "wss"
	}
	host := js.Global().Get("window").Get("location").Get("host").String()
	path := "/ws"
	wsURL := protocol + "://" + host + path
	ws := js.Global().Get("WebSocket").New(wsURL)
	if ws.IsUndefined() {
		log.Fatal("WebSocket not supported in this browser")
		return
	}

	log.Printf("Connected to WebSocket at %s\n", wsURL)

	ws.Call("addEventListener", "message", js.FuncOf(func(this js.Value, p []js.Value) interface{} { //nolint
		data := p[0].Get("data")
		if data.IsUndefined() {
			return nil
		}

		if data.Type() == js.TypeString {
			base64Data := data.String()
			bytes, err := base64.StdEncoding.DecodeString(base64Data)
			if err != nil {
				log.Println("Failed to decode base64 data:", err)
				return nil
			}
			processFloat32Data(bytes)
		} else {
			log.Printf("Received data of unexpected type: %s", data.Type().String())
		}

		return nil
	}))

	wskt = ws
}

func processFloat32Data(bytes []byte) {
	floatData := make([]float32, len(bytes)/4)
	for i := range floatData {
		floatData[i] = math.Float32frombits(uint32(bytes[i*4]) | uint32(bytes[i*4+1])<<8 | uint32(bytes[i*4+2])<<16 | uint32(bytes[i*4+3])<<24)
	}

	// Process the float data directly
	updateSpectrogram(floatData)
}

func updateSpectrogram(floatData []float32) {
	magnitudes := sg.ComputeFFT(floatData)
	newColumn := make([]byte, h*4)

	for y := 0; y < h; y++ {
		color := sg.MagnitudeToPixel(magnitudes[y])
		r, g, b, a := color.RGBA()
		newColumn[y*4+0] = byte(r >> 8)
		newColumn[y*4+1] = byte(g >> 8)
		newColumn[y*4+2] = byte(b >> 8)
		newColumn[y*4+3] = byte(a >> 8)
	}

	sHist[sHistIndex] = newColumn
	sHistIndex = (sHistIndex + 1) % historySize
}

func initGL() {
	doc := js.Global().Get("document")
	c := doc.Call("getElementById", "gocanvas")

	if c.IsUndefined() {
		log.Fatal("Canvas element with ID 'gocanvas' not found")
		return
	}

	w = 600
	h = 512
	c.Set("width", w)
	c.Set("height", h)
	gl = c.Call("getContext", "webgl")
	if gl.IsUndefined() {
		gl = c.Call("getContext", "experimental-webgl")
	}
	if gl.IsUndefined() {
		js.Global().Call("alert", "Browser might not support WebGL")
		return
	}
	glTypes.New(gl)
}

func initShaders() {
	vSh := compileShader(vShadSrc, glTypes.VertexShader)
	fSh := compileShader(fShadSrc, glTypes.FragmentShader)

	sP = gl.Call("createProgram")
	gl.Call("attachShader", sP, vSh)
	gl.Call("attachShader", sP, fSh)
	gl.Call("linkProgram", sP)

	if !gl.Call("getProgramParameter", sP, glTypes.LinkStatus).Bool() {
		log.Fatal("Could not initialize shaders: ", gl.Call("getProgramInfoLog", sP).String())
	}

	gl.Call("useProgram", sP)

	uSampler = gl.Call("getUniformLocation", sP, "uSampler")
	if uSampler.IsNull() {
		log.Fatal("Failed to get uniform location for uSampler")
	}

	vPos = gl.Call("getAttribLocation", sP, "position")
	if vPos.IsNull() {
		log.Fatal("Failed to get attribute location for position")
	}

	vTexCoord = gl.Call("getAttribLocation", sP, "texCoord")
	if vTexCoord.IsNull() {
		log.Fatal("Failed to get attribute location for texCoord")
	}
}

func compileShader(src string, sT js.Value) js.Value {
	s := gl.Call("createShader", sT)
	gl.Call("shaderSource", s, src)
	gl.Call("compileShader", s)

	if !gl.Call("getShaderParameter", s, glTypes.CompileStatus).Bool() {
		log.Fatal("Shader compilation failed: ", gl.Call("getShaderInfoLog", s).String())
	}

	return s
}

func spectexture() {
	t = gl.Call("createTexture")
	gl.Call("bindTexture", glTypes.Texture2D, t)
	gl.Call("texParameteri", glTypes.Texture2D, glTypes.TextureMinFilter, glTypes.Linear)
	gl.Call("texParameteri", glTypes.Texture2D, glTypes.TextureMagFilter, glTypes.Linear)
	gl.Call("texParameteri", glTypes.Texture2D, glTypes.TextureWrapS, glTypes.ClampToEdge)
	gl.Call("texParameteri", glTypes.Texture2D, glTypes.TextureWrapT, glTypes.ClampToEdge)
	gl.Call("texImage2D", glTypes.Texture2D, 0, glTypes.RGBA, w, h, 0, glTypes.RGBA, glTypes.UnsignedByte, js.Null())
}

func initHist() {
	sHist = make([][]byte, historySize)
	for i := range sHist {
		sHist[i] = make([]byte, h*4)
	}
	sHistIndex = 0
}

func renderLoop() {
	log.Println("Starting render loop")

	rndrFr = js.FuncOf(func(this js.Value, args []js.Value) interface{} { //nolint
		renderSpect()
		js.Global().Call("requestAnimationFrame", rndrFr)
		return nil
	})

	js.Global().Call("requestAnimationFrame", rndrFr)
}

func renderSpect() {
	gl.Call("clearColor", 0, 0, 0, 1)
	gl.Call("clear", glTypes.ColorBufferBit)
	checkGLError("Error clearing canvas")

	gl.Call("bindTexture", glTypes.Texture2D, t)
	checkGLError("Error binding texture")

	for i, column := range sHist {
		if len(column) != h*4 {
			log.Printf("Invalid column size: %d", len(column))
			continue
		}

		arrayBuffer := js.Global().Get("ArrayBuffer").New(len(column))
		uint8Array := js.Global().Get("Uint8Array").New(arrayBuffer)
		js.CopyBytesToJS(uint8Array, column)

		gl.Call("texSubImage2D", glTypes.Texture2D, 0, i, 0, 1, h, glTypes.RGBA, glTypes.UnsignedByte, uint8Array)
		checkGLError("Error updating texture data")
	}

	gl.Call("uniform1i", uSampler, 0)
	checkGLError("Error setting uniform sampler")

	qVData := []float32{
		-1, -1, 0,
		1, -1, 0,
		-1, 1, 0,
		1, 1, 0,
	}
	qTCData := []float32{
		0, 0,
		1, 0,
		0, 1,
		1, 1,
	}

	vBuff := gl.Call("createBuffer")
	gl.Call("bindBuffer", glTypes.ArrayBuffer, vBuff)
	gl.Call("bufferData", glTypes.ArrayBuffer, wgl.SliceToTypedArray(qVData), glTypes.StaticDraw)
	checkGLError("Error creating or binding vertex buffer")
	gl.Call("enableVertexAttribArray", vPos)
	gl.Call("vertexAttribPointer", vPos, 3, glTypes.Float, false, 0, 0)
	checkGLError("Error setting vertex attribute pointer")

	tCBuff := gl.Call("createBuffer")
	gl.Call("bindBuffer", glTypes.ArrayBuffer, tCBuff)
	gl.Call("bufferData", glTypes.ArrayBuffer, wgl.SliceToTypedArray(qTCData), glTypes.StaticDraw)
	checkGLError("Error creating or binding texture coordinate buffer")
	gl.Call("enableVertexAttribArray", vTexCoord)
	gl.Call("vertexAttribPointer", vTexCoord, 2, glTypes.Float, false, 0, 0)
	checkGLError("Error setting texture coordinate attribute pointer")

	gl.Call("drawArrays", glTypes.TriangleStrip, 0, 4)
	checkGLError("Error drawing arrays")
}

func checkGLError(stage string) {
	err := gl.Call("getError").Int()
	if err != 0 {
		log.Printf("WebGL Error during %s: %d", stage, err)
	}
}

const vShadSrc = `
attribute vec3 position;
attribute vec2 texCoord;
varying vec2 vTexCoord;
void main(void) {
	gl_Position = vec4(position, 1.0);
	vTexCoord = texCoord;
}`

const fShadSrc = `
precision mediump float;
varying vec2 vTexCoord;
uniform sampler2D uSampler;
void main(void) {
	gl_FragColor = texture2D(uSampler, vTexCoord);
}`
