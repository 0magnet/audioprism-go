// Package ui pkg/ui/tcell/ui.go
package ui

import (
	"image/color"
	"log"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/jfreymuth/pulse"
	"golang.org/x/net/websocket"

	"github.com/0magnet/audioprism-go/pkg/spectrogram"
	"github.com/0magnet/audioprism-go/pkg/wsaudio/wscodec"
)

var (
	spectrogramHistory [][]color.Color
	historyIndex       int
	termWidth          int
	termHeight         int
	bufferWidth        int // Fixed circular buffer size
	bufferHeight       int // Fixed frequency bin count
	screen             tcell.Screen
	sampleBuffer       []float32
	overlapSamples     []float32
	audioChan          chan []float32
	historyMutex       sync.RWMutex

	// Column queue for smooth scrolling
	columnQueue      [][]color.Color
	columnQueueMutex sync.Mutex
)

// Run initializes and starts the Tcell application
func Run(wid, hei, fpsRate, bSize int, fpsDisp bool, wsURL string) { //nolint:revive
	var err error
	screen, err = tcell.NewScreen()
	if err != nil {
		log.Fatalf("failed to create screen: %v", err)
	}
	if err = screen.Init(); err != nil {
		log.Fatalf("failed to initialize screen: %v", err)
	}
	defer screen.Fini()

	termWidth, termHeight = screen.Size()

	// Fixed buffer sizes (independent of terminal size)
	bufferWidth = 2048
	bufferHeight = 1024

	spectrogramHistory = make([][]color.Color, bufferWidth)
	historyIndex = 0
	for i := range spectrogramHistory {
		spectrogramHistory[i] = make([]color.Color, bufferHeight)
		for j := range spectrogramHistory[i] {
			spectrogramHistory[i][j] = color.Black
		}
	}

	// Create channel for audio data (buffered to prevent blocking)
	audioChan = make(chan []float32, 100)

	// Start processing goroutine
	go processAudioThread()

	var stream *pulse.RecordStream
	var audioCtx *pulse.Client
	if wsURL == "" {
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

	go renderLoop(fpsDisp)
	if stream != nil {
		stream.Start()
		defer stream.Stop()
	}

	for {
		ev := screen.PollEvent()
		switch ev := ev.(type) {
		case *tcell.EventKey:
			if ev.Key() == tcell.KeyEscape || ev.Key() == tcell.KeyCtrlC {
				return
			}
		case *tcell.EventResize:
			termWidth, termHeight = screen.Size()
			screen.Sync()
		}
	}
}

func renderLoop(fpsDisp bool) {
	startTime := time.Now()
	var framecount int
	var fps float64

	// Render at ~60fps, drain all pending columns each frame
	ticker := time.NewTicker(time.Duration(1000/60) * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		// Drain ALL pending columns (like original audioprism)
		columnQueueMutex.Lock()
		for len(columnQueue) > 0 {
			historyMutex.Lock()
			spectrogramHistory[historyIndex] = columnQueue[0]
			historyIndex = (historyIndex + 1) % bufferWidth
			historyMutex.Unlock()
			columnQueue = columnQueue[1:]
		}
		columnQueueMutex.Unlock()

		screen.Clear()
		drawSpectrogram()

		if fpsDisp {
			framecount++
			if time.Since(startTime) > 2*time.Second {
				fps = float64(framecount) / time.Since(startTime).Seconds()
				startTime = time.Now()
				framecount = 0
			}
			fpsStr := "FPS: " + strconv.FormatFloat(fps, 'f', 2, 64)
			drawText(screen, termWidth-len(fpsStr)-2, 0, fpsStr, tcell.StyleDefault.Foreground(tcell.ColorRed))
		}

		screen.Show()
	}
}

func drawSpectrogram() {
	localW := termWidth
	localH := termHeight

	historyMutex.RLock()
	localIndex := historyIndex
	// Show most recent localW columns from circular buffer
	startIndex := (localIndex - localW + bufferWidth) % bufferWidth
	for x := 0; x < localW; x++ {
		index := (startIndex + x) % bufferWidth
		for y := 0; y < localH; y++ {
			// Scale from fixed buffer height to terminal height
			bufferY := int(float64(y) / float64(localH) * float64(bufferHeight))
			if bufferY < bufferHeight {
				col := spectrogramHistory[index][bufferY]
				drawCell(screen, x, localH-1-y, col)
			}
		}
	}
	historyMutex.RUnlock()
}

func drawCell(s tcell.Screen, x, y int, col color.Color) {
	r, g, b, _ := col.RGBA()
	style := tcell.StyleDefault.Background(tcell.NewRGBColor(int32(r>>8), int32(g>>8), int32(b>>8))) //nolint
	s.SetContent(x, y, ' ', nil, style)
}

func drawText(s tcell.Screen, x, y int, text string, style tcell.Style) {
	for i, r := range text {
		s.SetContent(x+i, y, r, nil, style)
	}
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

// Processing goroutine - pushes columns to queue
func processAudioThread() {
	fftSize := spectrogram.S.GetDFTSize()
	overlapSamples = make([]float32, fftSize)

	for samples := range audioChan {
		sampleBuffer = append(sampleBuffer, samples...)

		newSize := spectrogram.S.GetDFTSize()
		if newSize != fftSize {
			fftSize = newSize
			overlapSamples = make([]float32, fftSize)
			sampleBuffer = nil
			continue
		}

		stepSize := spectrogram.S.StepSize()
		for len(sampleBuffer) >= stepSize {
			copy(overlapSamples, overlapSamples[stepSize:])
			copy(overlapSamples[stepSize:], sampleBuffer[:fftSize-stepSize])
			sampleBuffer = sampleBuffer[stepSize:]

			magnitudes := spectrogram.ComputeFFT(overlapSamples)

			currentRow := make([]color.Color, bufferHeight)
			for y := 0; y < bufferHeight; y++ {
				freq := float64(y) / float64(bufferHeight) * 12000
				bin := int(freq * float64(fftSize) / float64(spectrogram.SampleRate))
				if bin < len(magnitudes) {
					currentRow[y] = spectrogram.MagnitudeToPixel(magnitudes[bin])
				} else {
					currentRow[y] = color.Black
				}
			}

			columnQueueMutex.Lock()
			columnQueue = append(columnQueue, currentRow)
			if len(columnQueue) > 200 {
				columnQueue = columnQueue[len(columnQueue)-100:]
			}
			columnQueueMutex.Unlock()
		}
	}
}
