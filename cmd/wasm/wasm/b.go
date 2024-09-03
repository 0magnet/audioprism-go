//go:build js && wasm

// Package main cmd/wasm/wasm/b.go
package main

import (
	"encoding/base64"
	"log"
	"math"
	"strconv"
	"syscall/js"

	sg "github.com/0magnet/audioprism-go/pkg/spectrogram"
	"github.com/0magnet/audioprism-go/pkg/wgl"
)

var (
	width, height                                             string
	w                                                         = 512
	h                                                         = 512
	gl, t, wskt, sP, uSampler, vPos, vTexCoord, vBuff, tCBuff js.Value //nolint:unused
	sgHist                                                    [][]byte
	sgHistIndex                                               int
	rndrFr                                                    js.Func
	glTypes                                                   wgl.GLTypes
	historySize                                               int
	startTime                                                 = js.Global().Get("performance").Call("now").Float()
	frameCount                                                int
	fps                                                       float64
	fpsDisplay                                                js.Value
)

func main() {
	if width != "" {
		num, err := strconv.Atoi(width)
		if err != nil {
			log.Println("error parsing width: ", err)
		} else {
			w = num
		}
	}
	if height != "" {
		num, err := strconv.Atoi(height)
		if err != nil {
			log.Println("error parsing height: ", err)
		} else {
			h = num
		}
	}
	historySize = w

	initWS()
	initGL()
	initShaders()
	spectexture()
	initHist()
	initBuffers()
	createFPSDisplay()
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
			b, err := base64.StdEncoding.DecodeString(data.String())
			if err != nil {
				log.Println("Failed to decode base64 data:", err)
				return nil
			}
			floatData := make([]float32, len(b)/4)
			for i := range floatData {
				floatData[i] = math.Float32frombits(uint32(b[i*4]) |
					uint32(b[i*4+1])<<8 |
					uint32(b[i*4+2])<<16 |
					uint32(b[i*4+3])<<24)
			}
			_, err = processAudio(floatData)
			if err != nil {
				log.Fatal(err)
			}
		} else {
			log.Printf("Received data of unexpected type: %s", data.Type().String())
		}

		return nil
	}))

	wskt = ws
}

func processAudio(p []float32) (int, error) { //nolint
	start := 0
	step := sg.FFTSize / 2
	for len(p)-start >= sg.FFTSize {
		magnitudes := sg.ComputeFFT(p[start : start+sg.FFTSize])
		start += step
		newColumn := make([]byte, h*4)
		for y := 0; y < h; y++ {
			freq := float64(y) / float64(h) * 12000
			color := sg.MagnitudeToPixel(magnitudes[int(freq*float64(sg.FFTSize)/44100)])
			r, g, b, a := color.RGBA()
			newColumn[y*4+0] = byte(r >> 8)
			newColumn[y*4+1] = byte(g >> 8)
			newColumn[y*4+2] = byte(b >> 8)
			newColumn[y*4+3] = byte(a >> 8)
		}

		sgHist[sgHistIndex] = newColumn
		sgHistIndex = (sgHistIndex + 1) % historySize
	}
	return len(p), nil
}

func initGL() {
	doc := js.Global().Get("document")
	c := doc.Call("getElementById", "gocanvas")

	if c.IsUndefined() {
		log.Fatal("Canvas element with ID 'gocanvas' not found")
		return
	}

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
	sgHist = make([][]byte, historySize)
	for i := range sgHist {
		sgHist[i] = make([]byte, h*4)
	}
	sgHistIndex = 0
}

func createFPSDisplay() {
	doc := js.Global().Get("document")
	fpsDisplay = doc.Call("createElement", "div")
	fpsDisplay.Set("id", "fpsDisplay")
	fpsDisplay.Get("style").Set("position", "absolute")
	fpsDisplay.Get("style").Set("bottom", "10px")
	fpsDisplay.Get("style").Set("left", "10px")
	fpsDisplay.Get("style").Set("color", "white")
	fpsDisplay.Set("innerHTML", "FPS: 0")
	doc.Get("body").Call("appendChild", fpsDisplay)
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

func setupVertexAttribs() {
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
	if vBuff.IsUndefined() {
		vBuff = gl.Call("createBuffer")
		gl.Call("bindBuffer", glTypes.ArrayBuffer, vBuff)
		gl.Call("bufferData", glTypes.ArrayBuffer, wgl.SliceToTypedArray(qVData), glTypes.StaticDraw)
		checkGLError("Error creating or binding vertex buffer")
	}
	if tCBuff.IsUndefined() {
		tCBuff = gl.Call("createBuffer")
		gl.Call("bindBuffer", glTypes.ArrayBuffer, tCBuff)
		gl.Call("bufferData", glTypes.ArrayBuffer, wgl.SliceToTypedArray(qTCData), glTypes.StaticDraw)
		checkGLError("Error creating or binding texture coordinate buffer")
	}
	gl.Call("bindBuffer", glTypes.ArrayBuffer, vBuff)
	gl.Call("enableVertexAttribArray", vPos)
	gl.Call("vertexAttribPointer", vPos, 3, glTypes.Float, false, 0, 0)
	checkGLError("Error setting vertex attribute pointer")

	gl.Call("bindBuffer", glTypes.ArrayBuffer, tCBuff)
	gl.Call("enableVertexAttribArray", vTexCoord)
	gl.Call("vertexAttribPointer", vTexCoord, 2, glTypes.Float, false, 0, 0)
	checkGLError("Error setting texture coordinate attribute pointer")
}

var uint8Array js.Value

func initBuffers() {
	arrayBuffer := js.Global().Get("ArrayBuffer").New(w * h * 4)
	uint8Array = js.Global().Get("Uint8Array").New(arrayBuffer)
}

func renderSpect() {
	gl.Call("clearColor", 0, 0, 0, 1)
	gl.Call("clear", glTypes.ColorBufferBit)
	checkGLError("Error clearing canvas")
	gl.Call("bindTexture", glTypes.Texture2D, t)
	checkGLError("Error binding texture")

	//	/*
	//reduce texSubImage2D usage
	fullTextureData := make([]byte, w*h*4)
	for x := 0; x < len(sgHist); x++ {
		index := (sgHistIndex + x) % len(sgHist)
		copy(fullTextureData[x*h*4:(x+1)*h*4], sgHist[index])
	}
	js.CopyBytesToJS(uint8Array, fullTextureData)
	gl.Call("texSubImage2D", glTypes.Texture2D, 0, 0, 0, w, h, glTypes.RGBA, glTypes.UnsignedByte, uint8Array)
	checkGLError("Error updating texture data")
	//	*/
	/*

		//original method - slow
			for x := 0; x < len(sgHist); x++ {
				index := (sgHistIndex + x) % len(sgHist)
				arrayBuffer := js.Global().Get("ArrayBuffer").New(len(sgHist[index]))
				uint8Array := js.Global().Get("Uint8Array").New(arrayBuffer)
				js.CopyBytesToJS(uint8Array, sgHist[index])
				gl.Call("texSubImage2D", glTypes.Texture2D, 0, x, 0, 1, len(sgHist[index])/4, glTypes.RGBA, glTypes.UnsignedByte, uint8Array)
				checkGLError("Error updating texture data")
			}
	*/
	gl.Call("uniform1i", uSampler, 0)
	checkGLError("Error setting uniform sampler")
	setupVertexAttribs()
	gl.Call("drawArrays", glTypes.TriangleStrip, 0, 4)
	checkGLError("Error drawing arrays")
	updateFPSDisplay()
}

func updateFPSDisplay() {
	frameCount++
	currentTime := js.Global().Get("performance").Call("now").Float()
	elapsedTime := currentTime - startTime
	if elapsedTime > 2000 {
		fps = float64(frameCount) / (elapsedTime / 1000.0)
		startTime = currentTime
		frameCount = 0
		fpsDisplay.Set("innerHTML", "FPS: "+strconv.FormatFloat(fps, 'f', 2, 64))
	}
}

func checkGLError(stage string) {
	err := gl.Call("getError").Int()
	if err != 0 {
		log.Printf("WebGL Error during %s: %d", stage, err)
	}
}

const vShadSrc = `
attribute vec4 position;
attribute vec2 texCoord;
varying vec2 vTexCoord;

void main() {
    gl_Position = position;
	vTexCoord = vec2(texCoord.y, texCoord.x);
}

`

const fShadSrc = `
precision mediump float;
varying vec2 vTexCoord;
uniform sampler2D uSampler;
void main(void) {
	gl_FragColor = texture2D(uSampler, vTexCoord);
}`
