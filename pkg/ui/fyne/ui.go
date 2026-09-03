// Package ui pkg/ui/fyne/ui.go
package ui

import (
	"image"
	"image/color"
	"image/draw"
	"log"
	"net/url"
	"strconv"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"github.com/jfreymuth/pulse"
	"golang.org/x/net/websocket"

	"github.com/0magnet/audioprism-go/pkg/spectrogram"
	"github.com/0magnet/audioprism-go/pkg/wsaudio/wscodec"
)

var (
	spectrogramHistory [][]color.Color
	historyIndex       int
	width, height      int
	bufferWidth        int // Fixed buffer size
	bufferHeight       int
	audioSamples       []float32
	overlapSamples     []float32
	audioChan          chan []float32
	columnChan         chan []color.Color
	historyMutex       sync.RWMutex
	sizeMutex          sync.RWMutex
	columnBuffer       [][]color.Color
	columnBufferMutex  sync.Mutex
)

func allocateSpectrogramHistory() {
	// Use fixed buffer size (larger than typical window)
	bufferWidth = 2048
	bufferHeight = 1024

	spectrogramHistory = make([][]color.Color, bufferWidth)
	for i := range spectrogramHistory {
		spectrogramHistory[i] = make([]color.Color, bufferHeight)
		for j := range spectrogramHistory[i] {
			spectrogramHistory[i][j] = color.Black
		}
	}
	historyIndex = 0
}

// Run initializes and starts the Fyne application
func Run(wid, hei, fpsRate, bSize int, fpsDisp bool, wsURL string) { //nolint:revive
	width = wid
	height = hei
	a := app.New()
	w := a.NewWindow("audioprism-fyne")
	allocateSpectrogramHistory()

	// Create channel for audio data (buffered to prevent blocking)
	audioChan = make(chan []float32, 100)

	// Create channel for computed columns (buffered)
	columnChan = make(chan []color.Color, 100)

	// Start processing goroutine (computes FFT)
	go processAudioThread()

	// Start goroutine to buffer columns
	go func() {
		for row := range columnChan {
			columnBufferMutex.Lock()
			columnBuffer = append(columnBuffer, row)
			// If buffer gets too large (>20), skip ahead to stay responsive
			if len(columnBuffer) > 20 {
				columnBuffer = columnBuffer[len(columnBuffer)-10:]
			}
			columnBufferMutex.Unlock()
		}
	}()

	img := canvas.NewRaster(func(w, h int) image.Image {
		rgba := image.NewRGBA(image.Rect(0, 0, width, height))
		draw.Draw(rgba, rgba.Bounds(), &image.Uniform{color.Black}, image.Point{}, draw.Src)

		historyMutex.RLock()
		sizeMutex.RLock()
		localIndex := historyIndex
		localWidth := width
		localHeight := height
		sizeMutex.RUnlock()

		// Show most recent localWidth columns (1:1 pixel-to-column mapping)
		startIndex := (localIndex - localWidth + bufferWidth) % bufferWidth
		for x := 0; x < localWidth; x++ {
			index := (startIndex + x) % bufferWidth
			for y := 0; y < localHeight; y++ {
				// Scale height from buffer
				bufferY := int(float64(y) / float64(localHeight) * float64(bufferHeight))
				if bufferY < bufferHeight {
					rgba.Set(x, localHeight-1-y, spectrogramHistory[index][bufferY])
				}
			}
		}
		historyMutex.RUnlock()
		return rgba
	})

	fpsText := canvas.NewText("FPS: 0", color.RGBA{255, 0, 0, 255})
	fpsText.Alignment = fyne.TextAlignTrailing
	overlay := container.NewWithoutLayout(fpsText)
	mainContainer := container.NewStack(img, overlay)
	w.SetContent(mainContainer)

	// Keyboard controls (matching original audioprism)
	w.Canvas().SetOnTypedKey(func(ev *fyne.KeyEvent) {
		switch ev.Name {
		case fyne.KeyC:
			spectrogram.S.CycleColor()
		case fyne.KeyL:
			spectrogram.S.ToggleScale()
		case fyne.KeyMinus:
			spectrogram.S.AdjustMin(-1.0)
		case fyne.KeyEqual:
			spectrogram.S.AdjustMin(1.0)
		case fyne.KeyLeftBracket:
			spectrogram.S.AdjustMax(-1.0)
		case fyne.KeyRightBracket:
			spectrogram.S.AdjustMax(1.0)
		case fyne.KeyLeft:
			spectrogram.S.HalveFFTSize()
		case fyne.KeyRight:
			spectrogram.S.DoubleFFTSize()
		case fyne.KeyDown:
			spectrogram.S.AdjustOverlap(-0.01)
		case fyne.KeyUp:
			spectrogram.S.AdjustOverlap(0.01)
		case fyne.KeyQ:
			w.Close()
		}
	})
	w.Canvas().SetOnTypedRune(func(r rune) {
		if r == 'w' {
			spectrogram.S.CycleWindow()
		}
	})

	// Render at high frame rate (~200fps like original audioprism)
	// Drain all pending columns each frame — no audio clock sync needed
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()

	// Setup audio or websocket
	var stream *pulse.RecordStream
	var audioCtx *pulse.Client
	if wsURL == "" {
		var err error
		audioCtx, err = pulse.NewClient()
		if err != nil {
			log.Fatal(err)
		}
		defer audioCtx.Close()

		stream, err = audioCtx.NewRecord(pulse.Float32Writer(processAudio), pulse.RecordSampleRate(spectrogram.SampleRate), pulse.RecordLatency(0.1))
		if err != nil {
			log.Fatal(err)
		}
		defer func() {
			if stream != nil {
				stream.Stop()
			}
		}()
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
					if err == websocket.ErrBadFrame {
						return
					}
					continue
				}

				_, err = processAudio(floatData)
				if err != nil {
					log.Fatal(err)
				}
			}
		}()
	}

	go func() {
		startTime := time.Now()
		var framecount int
		var fps float64

		for range ticker.C {
			// Drain ALL pending columns (like original audioprism)
			columnBufferMutex.Lock()
			for len(columnBuffer) > 0 {
				row := columnBuffer[0]
				columnBuffer = columnBuffer[1:]
				columnBufferMutex.Unlock()

				historyMutex.Lock()
				spectrogramHistory[historyIndex] = row
				historyIndex = (historyIndex + 1) % bufferWidth
				historyMutex.Unlock()

				columnBufferMutex.Lock()
			}
			columnBufferMutex.Unlock()

			fyne.Do(func() {
				currentSize := mainContainer.Size()
				newWidth := int(currentSize.Width)
				newHeight := int(currentSize.Height)

				if newWidth != width || newHeight != height {
					sizeMutex.Lock()
					width = newWidth
					height = newHeight
					sizeMutex.Unlock()
				}

				img.Refresh()

				if fpsDisp {
					framecount++
					if time.Since(startTime) > 2*time.Second {
						fps = float64(framecount) / time.Since(startTime).Seconds()
						startTime = time.Now()
						framecount = 0

						fpsText.Text = "FPS: " + strconv.FormatFloat(fps, 'f', 2, 64)
						fpsText.Refresh()
					}
					fpsText.Move(fyne.NewPos(mainContainer.Size().Width-fpsText.MinSize().Width-10, 10))
				}
			})
		}
	}()

	w.Resize(fyne.NewSize(float32(width), float32(height)))
	if stream != nil {
		stream.Start()
		defer stream.Stop()
	}
	w.ShowAndRun()
}

// Audio callback - just send to channel (like C++ AudioThread)
func processAudio(p []float32) (int, error) {
	// Make a copy to avoid data races
	samples := make([]float32, len(p))
	copy(samples, p)

	// Non-blocking send
	select {
	case audioChan <- samples:
	default:
		// Drop samples if channel full (prevents blocking audio thread)
	}

	return len(p), nil
}

// Processing goroutine - computes columns and sends to queue
func processAudioThread() {
	fftSize := spectrogram.S.GetDFTSize()
	overlapSamples = make([]float32, fftSize)

	for samples := range audioChan {
		audioSamples = append(audioSamples, samples...)

		// Check for dynamic DFT size changes
		newSize := spectrogram.S.GetDFTSize()
		if newSize != fftSize {
			fftSize = newSize
			overlapSamples = make([]float32, fftSize)
			audioSamples = nil
			continue
		}

		stepSize := spectrogram.S.StepSize()
		for len(audioSamples) >= stepSize {
			copy(overlapSamples, overlapSamples[stepSize:])
			copy(overlapSamples[stepSize:], audioSamples[:fftSize-stepSize])
			audioSamples = audioSamples[stepSize:]

			magnitudes := spectrogram.ComputeFFT(overlapSamples)

			row := make([]color.Color, bufferHeight)
			for y := 0; y < bufferHeight; y++ {
				freq := float64(y) / float64(bufferHeight) * 12000
				bin := int(freq * float64(fftSize) / float64(spectrogram.SampleRate))
				if bin < len(magnitudes) {
					row[y] = spectrogram.MagnitudeToPixel(magnitudes[bin])
				} else {
					row[y] = color.Black
				}
			}

			select {
			case columnChan <- row:
			default:
			}
		}
	}
}
