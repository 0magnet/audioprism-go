// Package ui pkg/ui/tcell/ui.go
package ui

import (
	"encoding/base64"
	"image/color"
	"log"
	"math"
	"net/url"
	"strconv"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/jfreymuth/pulse"
	"golang.org/x/net/websocket"

	"github.com/0magnet/audioprism-go/pkg/spectrogram"
)

var (
	spectrogramHistory    [][]color.Color
	historyIndex          int
	termWidth, termHeight int
	width, height         int
	screen                tcell.Screen
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
	width = termWidth
	height = termHeight
	spectrogramHistory = make([][]color.Color, width)
	historyIndex = 0
	for i := range spectrogramHistory {
		spectrogramHistory[i] = make([]color.Color, height)
		for j := range spectrogramHistory[i] {
			spectrogramHistory[i][j] = color.Black
		}
	}

	ticker := time.NewTicker(time.Duration(1000/60) * time.Millisecond)
	defer ticker.Stop()

	var stream *pulse.RecordStream
	var audioCtx *pulse.Client
	if wsURL == "" {
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
				var encodedData string
				err := websocket.Message.Receive(ws, &encodedData)
				if err != nil {
					log.Println("Error receiving WebSocket data:", err)
					if err == websocket.ErrBadFrame {
						return
					}
					continue
				}

				data, err := base64.StdEncoding.DecodeString(encodedData)
				if err != nil {
					log.Println("Error decoding Base64 data:", err)
					continue
				}

				floatData := make([]float32, len(data)/4)
				for i := range floatData {
					floatData[i] = math.Float32frombits(uint32(data[i*4]) |
						uint32(data[i*4+1])<<8 |
						uint32(data[i*4+2])<<16 |
						uint32(data[i*4+3])<<24)
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
			width = termWidth
			height = termHeight
			screen.Sync()
		}
	}
}

func renderLoop(fpsDisp bool) {
	startTime := time.Now()
	var framecount int
	var fps float64
	for range time.Tick(time.Second / 60) {
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
	scaleX := float64(width) / float64(termWidth)
	scaleY := float64(height) / float64(termHeight)

	for x := 0; x < termWidth; x++ {
		index := (historyIndex + int(float64(x)*scaleX)) % width
		for y := 0; y < termHeight; y++ {
			scaledY := int(float64(y) * scaleY)
			if scaledY < height {
				col := spectrogramHistory[index][scaledY]
				drawCell(screen, x, termHeight-1-y, col)
			}
		}
	}
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
