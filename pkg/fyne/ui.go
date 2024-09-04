// Package fyneui pkg/fyne/ui.go
package fyneui

import (
	"encoding/base64"
	"image"
	"image/color"
	"image/draw"
	"log"
	"math"
	"net/url"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"github.com/jfreymuth/pulse"
	"golang.org/x/net/websocket"

	"github.com/0magnet/audioprism-go/pkg/spectrogram"
)

var (
	spectrogramHistory [][]color.Color
	historyIndex       int
	width, height      int
)

// Run initializes and starts the Fyne application
func Run(wid, hei, _, _ int, fpsDisp bool, wsURL string) {
	width = wid
	height = hei
	a := app.New()
	w := a.NewWindow("audioprism-go")
	spectrogramHistory = make([][]color.Color, width)
	historyIndex = 0
	for i := range spectrogramHistory {
		spectrogramHistory[i] = make([]color.Color, height)
		for j := range spectrogramHistory[i] {
			spectrogramHistory[i][j] = color.Black
		}
	}

	img := canvas.NewRaster(func(w, h int) image.Image {
		if w != width || h != height {
			width = w
			height = h
		}

		if len(spectrogramHistory) != width {
			spectrogramHistory = make([][]color.Color, width)
			for i := range spectrogramHistory {
				spectrogramHistory[i] = make([]color.Color, height)
				for j := range spectrogramHistory[i] {
					spectrogramHistory[i][j] = color.Black
				}
			}
			historyIndex = 0
		}

		rgba := image.NewRGBA(image.Rect(0, 0, w, h))
		draw.Draw(rgba, rgba.Bounds(), &image.Uniform{color.Black}, image.Point{}, draw.Src)

		for x := 0; x < width; x++ {
			index := (historyIndex + x) % width
			for y := 0; y < height; y++ {
				rgba.Set(x, h-1-y, spectrogramHistory[index][y])
			}
		}

		return rgba
	})

	var stream *pulse.RecordStream
	var audioCtx *pulse.Client
	if wsURL == "" {
		// Initialize PulseAudio client and stream
		var err error
		audioCtx, err = pulse.NewClient()
		if err != nil {
			log.Fatal(err)
		}
		defer audioCtx.Close()

		stream, err = audioCtx.NewRecord(pulse.Float32Writer(processAudio), pulse.RecordLatency(0.1))
		if err != nil {
			log.Fatal(err)
		}
		defer func() {
			if stream != nil {
				stream.Stop()
			}
		}()
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
					if err == websocket.ErrBadFrame {
						// If the connection is closed, exit the goroutine
						return
					}
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

	fpsText := canvas.NewText("FPS: 0", color.RGBA{255, 0, 0, 255})
	fpsText.Alignment = fyne.TextAlignTrailing
	overlay := container.NewWithoutLayout(fpsText)
	mainContainer := container.NewStack(img, overlay)
	w.SetContent(mainContainer)
	ticker := time.NewTicker(time.Duration(1000/60) * time.Millisecond)
	defer ticker.Stop()

	go func() {
		startTime := time.Now()
		var framecount int
		var fps float64
		if fpsDisp {
			fpsText.Move(fyne.NewPos(mainContainer.Size().Width-fpsText.MinSize().Width-10, 10))
		}
		for range ticker.C {
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
			}
		}
	}()

	w.Resize(fyne.NewSize(float32(width), float32(height)))
	if stream != nil {
		stream.Start()
		defer stream.Stop()
	}
	w.ShowAndRun()
}

func processAudio(p []float32) (int, error) {
	start := 0
	step := spectrogram.FFTSize / 2
	for len(p)-start >= spectrogram.FFTSize {
		magnitudes := spectrogram.ComputeFFT(p[start : start+spectrogram.FFTSize])
		start += step
		currentRow := make([]color.Color, height)
		for y := 0; y < height; y++ {
			freq := float64(y) / float64(height) * 12000
			bin := int(freq * float64(spectrogram.FFTSize) / 44100)
			if bin < len(magnitudes) {
				magnitude := magnitudes[bin]
				currentRow[y] = spectrogram.MagnitudeToPixel(magnitude)
			} else {
				currentRow[y] = color.Black
			}
		}

		spectrogramHistory[historyIndex] = currentRow
		historyIndex = (historyIndex + 1) % width
	}
	return len(p), nil
}
