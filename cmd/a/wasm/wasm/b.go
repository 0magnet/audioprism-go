//go:build js && wasm

// Package main cmd/wasm/wasm/b.go
package main

import (
	"encoding/base64"
	"log"
	"strconv"
	"strings"
	"syscall/js"
	"time"
	"unsafe"

	sg "github.com/0magnet/audioprism-go/pkg/spectrogram"
	"github.com/0magnet/audioprism-go/pkg/wgl"
	"github.com/0magnet/audioprism-go/pkg/wsaudio"
)

var (
	width, height                                                      string
	tinygo                                                             string
	showfps                                                            string
	w                                                                  = 640
	h                                                                  = 480
	gl, t, wskt, sP, uSampler, uOffset, vPos, vTexCoord, vBuff, tCBuff js.Value //nolint:unused
	rndrFr                                                             js.Func
	glTypes                                                            wgl.GLTypes
	startTime                                                          = time.Now()
	frameCount                                                         int
	fps                                                                float64
	fpsDisplay                                                         js.Value

	// Audio playback (AudioWorklet runs in its own thread; we push samples via postMessage)
	audioCtx     js.Value
	audioNode    js.Value
	audioReady   bool
	pendingAudio []float32

	// Overlap buffer for FFT (maintains state across WebSocket chunks)
	overlapSamples []float32
	sampleBuffer   []float32

	// Column queue: each column is tagged with the input-stream sample index it
	// represents (range [startSample, startSample+sg.StepSize)). renderSpect
	// drains a column only once the audio engine reports those samples have been
	// output. That binds the visible column to the audio sample currently leaving
	// the speaker, so jitter and rebuffering preserve sync without any heuristics.
	columnQueue []spectColumn

	// samplesProcessed: input-stream sample index of the next chunk to be FFT'd.
	// Incremented by sg.StepSize each time processAudio emits a column.
	samplesProcessed int64
	// samplesOut: input-stream sample index of the next sample the audio engine
	// will drain from its ring buffer. Advances only while the engine is actually
	// playing data (not during hysteresis silence). For ScriptProcessor it's
	// incremented in onaudioprocess; for AudioWorklet the worklet posts deltas
	// back to main thread.
	samplesOut int64
	// samplesOutAtTick / samplesOutTickContextTime: snapshot of (samplesOut,
	// audioCtx.currentTime) at the most recent audio engine update, used to
	// smooth the discrete batch reports (every 170ms for ScriptProcessor) into
	// a continuous estimate between rAF frames. audioPlaying is true only when
	// the most recent batch actually drained samples (vs outputting silence).
	samplesOutAtTick          int64
	samplesOutTickContextTime float64
	audioPlaying              bool
	// outputLatencySamples: HW + driver buffer (samples). Subtracted from
	// samplesOut so the displayed column corresponds to what's actually leaving
	// the speaker, not the sample the engine just finished producing.
	outputLatencySamples int64

	// Circular column index into the GPU texture: next column to overwrite.
	// Newest column is at (texCol-1) mod w; texCol/w is the shader scroll offset.
	texCol int

	// Fullscreen mode (from URL query param)
	isFullscreen bool
	canvas       js.Value
)

// spectColumn pairs an FFT column's pixel data with the input-stream sample
// index it was produced from. The render loop uses this to drain in lockstep
// with audio playback.
type spectColumn struct {
	data        []byte
	startSample int64
}

func getQueryParam(name string) string {
	params := js.Global().Get("URLSearchParams").New(
		js.Global().Get("window").Get("location").Get("search"),
	)
	return params.Call("get", name).String()
}

func main() {
	if tinygo != "" {
		sg.SetSingleThreaded()
	}
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

	// URL query params override: ?w=800&h=600 or ?fullscreen
	if qw := getQueryParam("w"); qw != "<null>" && qw != "" {
		if num, err := strconv.Atoi(qw); err == nil {
			w = num
		}
	}
	if qh := getQueryParam("h"); qh != "<null>" && qh != "" {
		if num, err := strconv.Atoi(qh); err == nil {
			h = num
		}
	}
	if fs := getQueryParam("fullscreen"); fs != "<null>" && fs != "" {
		isFullscreen = true
		w = js.Global().Get("window").Get("innerWidth").Int()
		h = js.Global().Get("window").Get("innerHeight").Int()
	}

	// ?delay=<seconds> overrides the audio jitter buffer size. The spectrogram
	// is sample-locked to audio playback and tracks automatically.
	if d := getQueryParam("delay"); d != "<null>" && d != "" {
		if v, err := strconv.ParseFloat(d, 64); err == nil && v > 0 {
			audioJitterSec = v
		}
	}

	applySettingsFromQuery()

	initAudioPlayback()

	// outputLatencySamples accounts for the OS/HW buffer between the audio
	// engine's destination and the actual speaker. Without this the spectrogram
	// would lead by ~50–200ms (browser-dependent) because the engine reports
	// samples consumed before they're audibly emitted.
	if !audioCtx.IsUndefined() {
		latSec := 0.0
		if v := audioCtx.Get("baseLatency"); !v.IsUndefined() && !v.IsNaN() {
			latSec += v.Float()
		}
		if v := audioCtx.Get("outputLatency"); !v.IsUndefined() && !v.IsNaN() {
			latSec += v.Float()
		}
		outputLatencySamples = int64(latSec * float64(sg.SampleRate))
		log.Printf("Audio jitter: %.3fs, output latency: %.3fs (%d samples)",
			audioJitterSec, latSec, outputLatencySamples)
	}
	// The WebSocket unless ?audio=wt asked for WebTransport, and the
	// WebSocket again if WebTransport cannot be had. See wt.go.
	initAudioTransport()
	initGL()
	initShaders()
	spectexture()
	if showfps != "" {
		createFPSDisplay()
	}
	wireKeyboard()
	if isFullscreen {
		js.Global().Get("window").Call("addEventListener", "resize", js.FuncOf(func(this js.Value, args []js.Value) interface{} { //nolint
			onResize()
			return nil
		}))
	}
	renderLoop()
	select {}
}

// audioJitterSec is the size of the audio jitter buffer in seconds. Smaller =
// lower mouth-to-ear latency but more vulnerable to network burstiness; larger
// = more delay but smoother on lossy/jittery links. Default targets near-real-
// time voice over typical home networks; raise via ?delay= for Skywire/etc.
var audioJitterSec = 0.25

// audioWorkletJS runs in the dedicated audio rendering thread.
// It maintains a ring buffer fed by postMessage() from the main thread and
// drains it on each process() call. A simple jitter buffer hides network
// burstiness: on startup or underrun we output silence until audioJitterSec
// of audio is queued, then play continuously until the next underrun.
// audioWorkletJS is built at startup so audioJitterSec (set from URL) is baked
// in. The worklet posts a {drained: N} message after each process() call so
// main thread can advance samplesOut in lockstep with actual audio output.
var audioWorkletJS string

func buildAudioWorkletJS() string {
	// Use Replace rather than Sprintf so the JS `%` modulo operators are not
	// interpreted as fmt verbs (e.g., `% this` was eating the `t` of `this`).
	tmpl := `
class AudioStreamProcessor extends AudioWorkletProcessor {
  constructor() {
    super();
    this.ringSize = __RING__;
    this.ring = new Float32Array(this.ringSize);
    this.readPos = 0;
    this.writePos = 0;
    this.target = __TARGET__;
    this.playing = false;
    this.port.onmessage = (e) => {
      const samples = e.data;
      const n = samples.length;
      for (let i = 0; i < n; i++) {
        this.ring[this.writePos] = samples[i];
        this.writePos = (this.writePos + 1) % this.ringSize;
        if (this.writePos === this.readPos) {
          this.readPos = (this.readPos + 1) % this.ringSize;
        }
      }
    };
  }
  process(inputs, outputs) {
    const out = outputs[0][0];
    const n = out.length;
    const avail = (this.writePos - this.readPos + this.ringSize) % this.ringSize;
    if (!this.playing) {
      if (avail >= this.target) {
        this.playing = true;
      } else {
        return true;
      }
    }
    let drained = 0;
    for (let i = 0; i < n; i++) {
      if (this.readPos !== this.writePos) {
        out[i] = this.ring[this.readPos];
        this.readPos = (this.readPos + 1) % this.ringSize;
        drained++;
      } else {
        this.playing = false;
        break;
      }
    }
    // Batch posts (every 4 process() calls = 4*128 samples ≈ 21ms) to keep
    // postMessage overhead low while still letting the main thread know the
    // current playing/silent state.
    this.batchDrained = (this.batchDrained || 0) + drained;
    this.batchCalls = (this.batchCalls || 0) + 1;
    if (this.batchCalls >= 4) {
      this.port.postMessage(this.batchDrained);
      this.batchDrained = 0;
      this.batchCalls = 0;
    }
    return true;
  }
}
registerProcessor('audio-stream-processor', AudioStreamProcessor);
`
	js := strings.ReplaceAll(tmpl, "__RING__", strconv.Itoa(sg.SampleRate*8))
	js = strings.ReplaceAll(js, "__TARGET__", strconv.Itoa(int(audioJitterSec*float64(sg.SampleRate))))
	return js
}

func initAudioPlayback() {
	ac := js.Global().Get("AudioContext")
	if ac.IsUndefined() {
		ac = js.Global().Get("webkitAudioContext")
	}
	if ac.IsUndefined() {
		log.Println("AudioContext not supported, audio playback disabled")
		return
	}
	// Match the WebSocket source rate (sg.SampleRate) to avoid resampling and the
	// underrun "chop" that comes from a slower producer feeding a faster consumer.
	audioCtx = newAudioContextAtRate(ac, sg.SampleRate)

	// AudioWorklet is gated to secure contexts (HTTPS or localhost). On LAN-IP
	// HTTP, audioWorklet is undefined; fall back to the legacy ScriptProcessor.
	if audioCtx.Get("audioWorklet").IsUndefined() {
		log.Println("AudioWorklet unavailable (insecure context); falling back to ScriptProcessorNode")
		initScriptProcessorFallback()
	} else {
		initAudioWorklet()
	}

	js.Global().Get("document").Call("addEventListener", "click", js.FuncOf(func(this js.Value, args []js.Value) interface{} { //nolint
		if audioCtx.Get("state").String() == "suspended" {
			audioCtx.Call("resume")
		}
		return nil
	}), map[string]interface{}{"once": true})
}

// newAudioContextAtRate constructs an AudioContext with the requested sample
// rate. Browsers throw NotSupportedError if the rate is outside their accepted
// range; in that case we fall back to a default-rate context.
func newAudioContextAtRate(ac js.Value, rate int) (ctx js.Value) {
	defer func() {
		if r := recover(); r != nil {
			log.Println("AudioContext rejected sampleRate, using default:", r)
			ctx = ac.New()
		}
	}()
	ctx = ac.New(map[string]interface{}{"sampleRate": rate})
	return
}

func initAudioWorklet() {
	audioWorkletJS = buildAudioWorkletJS()
	blobParts := js.Global().Get("Array").New(1)
	blobParts.SetIndex(0, audioWorkletJS)
	blobOpts := js.ValueOf(map[string]interface{}{"type": "application/javascript"})
	blob := js.Global().Get("Blob").New(blobParts, blobOpts)
	workletURL := js.Global().Get("URL").Call("createObjectURL", blob)

	nodeOpts := js.ValueOf(map[string]interface{}{
		"numberOfInputs":     0,
		"numberOfOutputs":    1,
		"outputChannelCount": []interface{}{1},
	})

	var thenFn, catchFn js.Func
	thenFn = js.FuncOf(func(this js.Value, args []js.Value) interface{} { //nolint
		audioNode = js.Global().Get("AudioWorkletNode").New(audioCtx, "audio-stream-processor", nodeOpts)
		audioNode.Set("onprocessorerror", js.FuncOf(func(this js.Value, args []js.Value) interface{} { //nolint
			log.Println("AudioWorklet onprocessorerror")
			return nil
		}))
		// Worklet posts an int (drained sample count) per process() call (0 if
		// silenced). Track the most recent playing tick so renderSpect can
		// smoothly extrapolate samplesOut between updates.
		audioNode.Get("port").Set("onmessage", js.FuncOf(func(this js.Value, args []js.Value) interface{} { //nolint
			drained := int64(args[0].Get("data").Int())
			samplesOut += drained
			if drained > 0 {
				samplesOutAtTick = samplesOut
				samplesOutTickContextTime = audioCtx.Get("currentTime").Float()
				audioPlaying = true
			} else {
				audioPlaying = false
			}
			return nil
		}))
		audioNode.Call("connect", audioCtx.Get("destination"))
		audioReady = true
		log.Println("AudioWorklet ready, sample rate:", audioCtx.Get("sampleRate").Float())
		if len(pendingAudio) > 0 {
			sendToWorklet(pendingAudio)
			pendingAudio = nil
		}
		js.Global().Get("URL").Call("revokeObjectURL", workletURL)
		thenFn.Release()
		catchFn.Release()
		return nil
	})
	catchFn = js.FuncOf(func(this js.Value, args []js.Value) interface{} { //nolint
		log.Println("AudioWorklet addModule failed:", args[0].Get("message").String())
		js.Global().Get("URL").Call("revokeObjectURL", workletURL)
		thenFn.Release()
		catchFn.Release()
		return nil
	})
	audioCtx.Get("audioWorklet").Call("addModule", workletURL).Call("then", thenFn).Call("catch", catchFn)
}

// Legacy ring buffer used only when ScriptProcessor fallback is active.
// spJitterTarget is the buffer level (in samples) we wait for before starting
// or restarting playback after an underrun. At sg.SampleRate=24000 that's ~1s,
// which absorbs typical Skywire/internet jitter without audible chop.
var (
	spRingBuf      []float32
	spReadPos      int
	spWritePos     int
	spRingSize     int
	spJitterTarget int
	spPlaying      bool
)

func initScriptProcessorFallback() {
	spRingSize = sg.SampleRate * 8
	spRingBuf = make([]float32, spRingSize)
	spJitterTarget = int(audioJitterSec * float64(sg.SampleRate))

	bufSize := 4096
	processor := audioCtx.Call("createScriptProcessor", bufSize, 0, 1)
	processor.Set("onaudioprocess", js.FuncOf(func(this js.Value, args []js.Value) interface{} { //nolint
		output := args[0].Get("outputBuffer").Call("getChannelData", 0)
		outLen := output.Get("length").Int()
		avail := (spWritePos - spReadPos + spRingSize) % spRingSize
		if !spPlaying {
			if avail >= spJitterTarget {
				spPlaying = true
			} else {
				for i := 0; i < outLen; i++ {
					output.SetIndex(i, 0)
				}
				return nil
			}
		}
		drained := 0
		for i := 0; i < outLen; i++ {
			if spReadPos != spWritePos {
				output.SetIndex(i, spRingBuf[spReadPos])
				spReadPos = (spReadPos + 1) % spRingSize
				drained++
			} else {
				spPlaying = false
				for ; i < outLen; i++ {
					output.SetIndex(i, 0)
				}
				break
			}
		}
		samplesOut += int64(drained)
		if drained > 0 {
			samplesOutAtTick = samplesOut
			samplesOutTickContextTime = audioCtx.Get("currentTime").Float()
			audioPlaying = true
		} else {
			audioPlaying = false
		}
		return nil
	}))
	processor.Call("connect", audioCtx.Get("destination"))
	audioReady = true
	log.Println("ScriptProcessor fallback ready, sample rate:", audioCtx.Get("sampleRate").Float())
	if len(pendingAudio) > 0 {
		sendToScriptProcessor(pendingAudio)
		pendingAudio = nil
	}
}

func sendToScriptProcessor(samples []float32) {
	for _, s := range samples {
		spRingBuf[spWritePos] = s
		spWritePos = (spWritePos + 1) % spRingSize
		if spWritePos == spReadPos {
			spReadPos = (spReadPos + 1) % spRingSize
		}
	}
}

// sendToWorklet posts a copy of samples to the audio worklet's ring buffer.
// The underlying ArrayBuffer is transferred (zero-copy) so the worklet thread
// takes ownership without re-serialization.
func sendToWorklet(samples []float32) {
	n := len(samples)
	if n == 0 || audioNode.IsUndefined() {
		return
	}
	ab := js.Global().Get("ArrayBuffer").New(n * 4)
	u8 := js.Global().Get("Uint8Array").New(ab)
	// Reinterpreting the float32 samples as bytes to hand to CopyBytesToJS,
	// which is the only way across without copying the buffer twice. The
	// length is exactly the four bytes per float32 that back the slice.
	bytes := unsafe.Slice((*byte)(unsafe.Pointer(&samples[0])), n*4) //nolint:gosec
	js.CopyBytesToJS(u8, bytes)
	f32 := js.Global().Get("Float32Array").New(ab)
	transferList := js.Global().Get("Array").New(1)
	transferList.SetIndex(0, ab)
	audioNode.Get("port").Call("postMessage", f32, transferList)
}

func enqueueAudio(samples []float32) {
	if audioCtx.IsUndefined() {
		return
	}
	if !audioReady {
		// Buffer up to ~2s of audio while the worklet is initializing.
		pendingAudio = append(pendingAudio, samples...)
		if len(pendingAudio) > 96000 {
			pendingAudio = pendingAudio[len(pendingAudio)-48000:]
		}
		return
	}
	if !audioNode.IsUndefined() {
		sendToWorklet(samples)
	} else {
		sendToScriptProcessor(samples)
	}
}

var (
	wsURL        string
	wsOnMsg      js.Func
	reconnecting bool
)

func initWS() {
	protocol := "ws"
	if js.Global().Get("window").Get("location").Get("protocol").String() == "https:" {
		protocol = "wss"
	}
	host := js.Global().Get("window").Get("location").Get("host").String()
	wsURL = protocol + "://" + host + "/ws"

	// Create the message handler once, reuse across reconnects
	wsOnMsg = js.FuncOf(func(this js.Value, p []js.Value) interface{} { //nolint
		floatData := decodeWSMessage(p[0].Get("data"))
		if len(floatData) == 0 {
			return nil
		}
		enqueueAudio(floatData)
		processAudio(floatData)
		return nil
	})

	connectWS()
}

// decodeWSMessage turns one WebSocket message into samples, accepting
// either encoding of the same bytes: an ArrayBuffer from a binary frame,
// or a string from the older base64 text frame.
//
// Branching on the JS type is the whole compatibility story — no version
// handshake, no query parameter, no flag day. The two encodings are
// distinguishable by construction (a text frame can never arrive as an
// ArrayBuffer), so this page works against a server of either vintage,
// and the servers may be upgraded whenever they are upgraded.
func decodeWSMessage(data js.Value) []float32 {
	if data.IsUndefined() || data.IsNull() {
		return nil
	}
	if data.Type() == js.TypeString {
		b, err := base64.StdEncoding.DecodeString(data.String())
		if err != nil {
			log.Println("Failed to decode base64 data:", err)
			return nil
		}
		return wsaudio.BytesToFloat32(b)
	}
	if !data.InstanceOf(js.Global().Get("ArrayBuffer")) {
		return nil // a Blob, if some browser ignored binaryType; not readable here
	}
	u8 := js.Global().Get("Uint8Array").New(data)
	b := make([]byte, u8.Length())
	js.CopyBytesToGo(b, u8)
	return wsaudio.BytesToFloat32(b)
}

func connectWS() {
	ws := js.Global().Get("WebSocket").New(wsURL)
	if ws.IsUndefined() {
		log.Println("WebSocket not supported in this browser")
		return
	}

	// Binary frames arrive as a Blob by default, and a Blob is only
	// readable asynchronously (arrayBuffer() returns a promise), which
	// would put a task-queue hop between every audio chunk and the
	// spectrogram. An ArrayBuffer is readable in the handler itself.
	ws.Set("binaryType", "arraybuffer")

	ws.Call("addEventListener", "open", js.FuncOf(func(this js.Value, args []js.Value) interface{} { //nolint
		log.Printf("WebSocket connected to %s\n", wsURL)
		reconnecting = false
		return nil
	}))

	ws.Call("addEventListener", "message", wsOnMsg)

	ws.Call("addEventListener", "close", js.FuncOf(func(this js.Value, args []js.Value) interface{} { //nolint
		code := 0
		reason := ""
		wasClean := false
		if len(args) > 0 {
			ev := args[0]
			if v := ev.Get("code"); !v.IsUndefined() {
				code = v.Int()
			}
			if v := ev.Get("reason"); !v.IsUndefined() {
				reason = v.String()
			}
			if v := ev.Get("wasClean"); !v.IsUndefined() {
				wasClean = v.Bool()
			}
		}
		log.Printf("WebSocket closed (code=%d wasClean=%v reason=%q), reconnecting in 2s...",
			code, wasClean, reason)
		scheduleReconnect(2000)
		return nil
	}))

	ws.Call("addEventListener", "error", js.FuncOf(func(this js.Value, args []js.Value) interface{} { //nolint
		log.Println("WebSocket error")
		// close event will fire after error, which triggers reconnect
		return nil
	}))

	wskt = ws
}

func scheduleReconnect(delayMs int) {
	if reconnecting {
		return
	}
	reconnecting = true
	js.Global().Call("setTimeout", js.FuncOf(func(this js.Value, args []js.Value) interface{} { //nolint
		reconnecting = false
		connectWS()
		return nil
	}), delayMs)
}

// wireKeyboard gives this frontend the original's interactive controls, with
// the same keys, so that muscle memory carries across:
//
//	c   cycle color scheme      w   cycle window function
//	l   cycle linear/log        -/= decrease/increase min magnitude
//	[/] decrease/increase max magnitude
//	←/→ decrease/increase DFT size
//	↓/↑ decrease/increase overlap
//
// The settings are read fresh every frame by processAudio, so nothing needs to
// be rebuilt or restarted when one of these fires. Quit, fullscreen and the
// three overlays are the browser's business or absent here, so they are not
// bound; everything that changes the picture is.
func wireKeyboard() {
	js.Global().Get("document").Call("addEventListener", "keydown",
		js.FuncOf(func(this js.Value, args []js.Value) interface{} { //nolint
			if len(args) == 0 {
				return nil
			}
			e := args[0]
			// Leave the page's own shortcuts alone when a modifier is held.
			if e.Get("ctrlKey").Bool() || e.Get("metaKey").Bool() || e.Get("altKey").Bool() {
				return nil
			}
			handled := true
			switch e.Get("key").String() {
			case "c":
				sg.S.CycleColor()
			case "w":
				sg.S.CycleWindow()
			case "l":
				sg.S.ToggleScale()
			case "-":
				sg.S.AdjustMin(-1)
			case "=":
				sg.S.AdjustMin(1)
			case "[":
				sg.S.AdjustMax(-1)
			case "]":
				sg.S.AdjustMax(1)
			case "ArrowLeft":
				sg.S.HalveFFTSize()
			case "ArrowRight":
				sg.S.DoubleFFTSize()
			case "ArrowDown":
				sg.S.AdjustOverlap(-0.01)
			case "ArrowUp":
				sg.S.AdjustOverlap(0.01)
			default:
				handled = false
			}
			if handled {
				e.Call("preventDefault")
				magMin, magMax := sg.S.MagWindow()
				log.Printf("%s, %s, %s %.0f..%.0f, DFT %d, overlap %.0f%%",
					sg.S.ColorName(), sg.S.WindowName(), sg.S.ScaleName(),
					magMin, magMax, sg.S.GetDFTSize(), sg.S.GetOverlap()*100)
			}
			return nil
		}))
}

// applySettingsFromQuery carries the original's command-line options in over
// the URL, which is the only channel this frontend has: the other four take
// them as flags, and a browser tab has no argv. The names are the original's,
// minus the leading dashes —
//
//	?colors=heat|blue|grayscale
//	?window=hann|hamming|bartlett|rectangular
//	?magnitude-scale=logarithmic|linear
//	?magnitude-min=<value>  ?magnitude-max=<value>
//	?dft-size=<power of two, 64-8192>
//	?overlap=<percentage, 5-95>
//
// Overlap is a percentage here, as it is on the original's --overlap, while the
// settings hold a ratio.
func applySettingsFromQuery() {
	str := func(name string) (string, bool) {
		v := getQueryParam(name)
		return v, v != "<null>" && v != ""
	}
	num := func(name string) (float64, bool) {
		v, ok := str(name)
		if !ok {
			return 0, false
		}
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			log.Printf("ignoring ?%s=%q: %v", name, v, err)
			return 0, false
		}
		return f, true
	}

	if v, ok := str("colors"); ok {
		sg.S.SetColorByName(v)
	}
	if v, ok := str("window"); ok {
		sg.S.SetWindowByName(v)
	}
	if v, ok := str("magnitude-scale"); ok {
		sg.S.SetScaleByName(v)
	}
	// Max before min, so that a pair given together cannot be rejected on the
	// way in for crossing a floor that is about to move.
	if v, ok := num("magnitude-max"); ok {
		sg.S.SetMagMax(v)
	}
	if v, ok := num("magnitude-min"); ok {
		sg.S.SetMagMin(v)
	}
	if v, ok := num("dft-size"); ok {
		sg.S.SetDFTSize(int(v))
	}
	if v, ok := num("overlap"); ok {
		sg.S.SetOverlap(v / 100.0)
	}
}

// fftSize is the DFT size this frontend is currently framing to. It tracks
// sg.S rather than the sg.FFTSize constant: every other frontend reads the
// settings, so --dft-size and --overlap did nothing here and only here, which
// is the sort of difference that gets blamed on the browser.
var fftSize int

func processAudio(p []float32) {
	if overlapSamples == nil {
		fftSize = sg.S.GetDFTSize()
		overlapSamples = make([]float32, fftSize)
	}

	sampleBuffer = append(sampleBuffer, p...)

	// A changed DFT size makes the samples held over from the last frame the
	// wrong length to keep, so the window restarts empty — as the other
	// frontends do it.
	if newSize := sg.S.GetDFTSize(); newSize != fftSize {
		fftSize = newSize
		overlapSamples = make([]float32, fftSize)
		sampleBuffer = nil
		return
	}

	stepSize := sg.S.StepSize()
	for len(sampleBuffer) >= stepSize && len(sampleBuffer) >= fftSize-stepSize {
		copy(overlapSamples, overlapSamples[stepSize:])
		copy(overlapSamples[stepSize:], sampleBuffer[:fftSize-stepSize])
		sampleBuffer = sampleBuffer[stepSize:]

		magnitudes := sg.ComputeFFT(overlapSamples)
		newColumn := make([]byte, h*4)
		for y := 0; y < h; y++ {
			freq := float64(y) / float64(h) * 12000
			bin := int(freq * float64(fftSize) / float64(sg.SampleRate))
			if bin < len(magnitudes) {
				color := sg.MagnitudeToPixel(magnitudes[bin])
				r, g, b, a := color.RGBA()
				newColumn[y*4+0] = byte(r >> 8 & 0xFF)
				newColumn[y*4+1] = byte(g >> 8 & 0xFF)
				newColumn[y*4+2] = byte(b >> 8 & 0xFF)
				newColumn[y*4+3] = byte(a >> 8 & 0xFF)
			}
		}
		columnQueue = append(columnQueue, spectColumn{
			data:        newColumn,
			startSample: samplesProcessed,
		})
		samplesProcessed += int64(stepSize)
		// Safety cap: if the audio engine never starts (user never enabled audio,
		// or browser blocked autoplay forever), don't grow unboundedly. Drop the
		// oldest in that case — the audio head will resync once it does start.
		if len(columnQueue) > 1000 {
			drop := len(columnQueue) - 500
			columnQueue = columnQueue[drop:]
		}
	}
}

func initGL() {
	doc := js.Global().Get("document")
	canvas = doc.Call("getElementById", "gocanvas")
	if canvas.IsUndefined() {
		log.Fatal("Canvas element with ID 'gocanvas' not found")
		return
	}
	canvas.Set("width", w)
	canvas.Set("height", h)
	gl = canvas.Call("getContext", "webgl")
	if gl.IsUndefined() {
		gl = canvas.Call("getContext", "experimental-webgl")
	}
	if gl.IsUndefined() {
		js.Global().Call("alert", "Browser might not support WebGL")
		return
	}
	glTypes.New(gl)
}

func onResize() {
	newW := js.Global().Get("window").Get("innerWidth").Int()
	newH := js.Global().Get("window").Get("innerHeight").Int()
	if newW == w && newH == h {
		return
	}
	w = newW
	h = newH

	canvas.Set("width", w)
	canvas.Set("height", h)

	// Reallocate the GPU texture, zero-filled, and reset the circular cursor.
	gl.Call("bindTexture", glTypes.Texture2D, t)
	zeroAB := js.Global().Get("ArrayBuffer").New(w * h * 4)
	zeroU8 := js.Global().Get("Uint8Array").New(zeroAB)
	gl.Call("texImage2D", glTypes.Texture2D, 0, glTypes.RGBA, w, h, 0, glTypes.RGBA, glTypes.UnsignedByte, zeroU8)
	texCol = 0

	// Resize the per-column upload buffer.
	columnAB := js.Global().Get("ArrayBuffer").New(h * 4)
	columnUint8 = js.Global().Get("Uint8Array").New(columnAB)

	// Drop any queued columns sized for the old height.
	columnQueue = nil

	gl.Call("viewport", 0, 0, w, h)
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
	uOffset = gl.Call("getUniformLocation", sP, "uOffset")
	if uOffset.IsNull() {
		log.Fatal("Failed to get uniform location for uOffset")
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
	zeroAB := js.Global().Get("ArrayBuffer").New(w * h * 4)
	zeroU8 := js.Global().Get("Uint8Array").New(zeroAB)
	gl.Call("texImage2D", glTypes.Texture2D, 0, glTypes.RGBA, w, h, 0, glTypes.RGBA, glTypes.UnsignedByte, zeroU8)
	columnAB := js.Global().Get("ArrayBuffer").New(h * 4)
	columnUint8 = js.Global().Get("Uint8Array").New(columnAB)
}

func createFPSDisplay() {
	doc := js.Global().Get("document")
	fpsDisplay = doc.Call("createElement", "div")
	fpsDisplay.Set("id", "fpsDisplay")
	fpsDisplay.Get("style").Set("position", "absolute")
	fpsDisplay.Get("style").Set("bottom", "10px")
	fpsDisplay.Get("style").Set("left", "10px")
	fpsDisplay.Get("style").Set("color", "white")
	fpsDisplay.Set("innerHTML", "Time: 00:00:00<br>FPS: 0<br>Click to enable audio")
	doc.Get("body").Call("appendChild", fpsDisplay)
}

func updateFPSDisplay() {
	frameCount++
	currentTime := time.Now()
	elapsedTime := currentTime.Sub(startTime).Seconds()
	if elapsedTime > 2 {
		fps = float64(frameCount) / elapsedTime
		startTime = currentTime
		frameCount = 0
		html := "Time: " + currentTime.Format("15:04:05") + "<br>FPS: " + strconv.FormatFloat(fps, 'f', 2, 64)
		// A transport that fell back is worth saying out loud. Without
		// this the page looks exactly as it would if ?audio=wt had
		// worked, which is the failure most likely to go unnoticed.
		if transportNotice != "" {
			html += "<br>" + transportNotice
		}
		fpsDisplay.Set("innerHTML", html)
	}
}

func renderLoop() {
	log.Println("Starting render loop")
	rndrFr = js.FuncOf(func(this js.Value, args []js.Value) interface{} { //nolint
		nowMs := args[0].Float() // requestAnimationFrame timestamp in ms
		renderSpect(nowMs)
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

// columnUint8 is the JS Uint8Array used to upload a single column (h*4 bytes)
// to the GPU texture. Re-used across frames to avoid per-frame allocations.
var columnUint8 js.Value

func renderSpect(_ float64) {
	// Sample-locked column drain: a column is shown only once the audio engine
	// reports its full sample range has been output (minus output latency, so
	// the column matches what the speaker is *emitting*, not what the engine
	// has produced internally). Sync is intrinsic to the relationship between
	// samplesProcessed and samplesOut.
	//
	// Between batch reports from the engine (every 170ms for ScriptProcessor,
	// 21ms for AudioWorklet), extrapolate the cursor at sampleRate × elapsed
	// AC time so columns scroll continuously instead of in batches.
	samplesEst := samplesOut
	if audioPlaying && !audioCtx.IsUndefined() && samplesOutTickContextTime > 0 {
		nowCtx := audioCtx.Get("currentTime").Float()
		elapsed := nowCtx - samplesOutTickContextTime
		if elapsed > 0 {
			// Cap extrapolation in case engine reports stop coming (rebuffer,
			// suspended context, etc.) so we don't run away.
			if elapsed > 0.25 {
				elapsed = 0.25
			}
			samplesEst = samplesOutAtTick + int64(elapsed*float64(sg.SampleRate))
		}
	}
	cutoff := samplesEst - outputLatencySamples
	stepSize := int64(sg.S.StepSize())
	drained := 0
	if cutoff > 0 && len(columnQueue) > 0 {
		gl.Call("bindTexture", glTypes.Texture2D, t)
		for drained < len(columnQueue) {
			col := columnQueue[drained]
			if col.startSample+stepSize > cutoff {
				break
			}
			if len(col.data) == h*4 {
				js.CopyBytesToJS(columnUint8, col.data)
				gl.Call("texSubImage2D", glTypes.Texture2D, 0, texCol, 0, 1, h, glTypes.RGBA, glTypes.UnsignedByte, columnUint8)
				texCol = (texCol + 1) % w
			}
			drained++
		}
		if drained > 0 {
			columnQueue = columnQueue[drained:]
		}
	}

	gl.Call("clearColor", 0, 0, 0, 1)
	gl.Call("clear", glTypes.ColorBufferBit)
	gl.Call("bindTexture", glTypes.Texture2D, t)
	gl.Call("uniform1i", uSampler, 0)
	// uOffset positions the rightmost displayed pixel at the latest written column.
	gl.Call("uniform1f", uOffset, float64(texCol)/float64(w))
	setupVertexAttribs()
	gl.Call("drawArrays", glTypes.TriangleStrip, 0, 4)
	if showfps != "" {
		updateFPSDisplay()
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
	vTexCoord = texCoord;
}

`

// uOffset scrolls the texture horizontally so the newest column always lands
// at the right edge. mod() wraps around the circular column buffer, which lets
// us upload only the columns that changed instead of repacking the whole
// texture each frame.
const fShadSrc = `
precision mediump float;
varying vec2 vTexCoord;
uniform sampler2D uSampler;
uniform float uOffset;
void main(void) {
	float texX = mod(vTexCoord.x + uOffset, 1.0);
	gl_FragColor = texture2D(uSampler, vec2(texX, vTexCoord.y));
}`
