// Package fyneui implements a UI for visualizing a spectrogram using the Fyne GUI library.
package fyneui

import (
	"image"
	"image/color"
	"image/draw"
	"log"
	"strconv"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"github.com/jfreymuth/pulse"

	"github.com/0magnet/audioprism-go/pkg/spectrogram"
)

//var updateRate int

// Run initializes and starts the Fyne application for spectrogram visualization.
// It sets up audio recording, updates the spectrogram, and displays the result in a window.
func Run(_, bufferSize int) {
//	updateRate = upd
	c, err := pulse.NewClient()
	if err != nil {
		log.Fatal(err.Error())
	}
	defer c.Close()

	var audioBufferLock sync.Mutex

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

	a := app.New()
	w := a.NewWindow("audioprism-go")

	var spectrogramHistory [][]color.Color
	var currentWidth, currentHeight int
	var historyIndex int

	img := canvas.NewRaster(func(w, h int) image.Image {
		if w != currentWidth || h != currentHeight {
			currentWidth = w
			currentHeight = h

			spectrogramHistory = make([][]color.Color, currentWidth)
			for i := range spectrogramHistory {
				spectrogramHistory[i] = make([]color.Color, currentHeight)
				for j := range spectrogramHistory[i] {
					spectrogramHistory[i][j] = color.Black
				}
			}
			historyIndex = 0
		}

		rgba := image.NewRGBA(image.Rect(0, 0, w, h))
		draw.Draw(rgba, rgba.Bounds(), &image.Uniform{color.Black}, image.Point{}, draw.Src)

		for x := 0; x < currentWidth; x++ {
			for y := 0; y < currentHeight; y++ {
				rgba.Set(x, h-1-y, spectrogramHistory[(x+historyIndex)%currentWidth][y])
			}
		}

		return rgba
	})

	fpsText := canvas.NewText("FPS: 0", color.RGBA{255, 0, 0, 255})
	fpsText.Alignment = fyne.TextAlignTrailing

	overlay := container.NewWithoutLayout(fpsText)

	// Replace deprecated container.NewMax with container.NewStack
	mainContainer := container.NewStack(img, overlay)
	w.SetContent(mainContainer)

	ticker := time.NewTicker(time.Duration(1000/120) * time.Millisecond)
		 defer ticker.Stop()

	go func() {
		startTime := time.Now()
		var framecount int
		var fps float64
		for range ticker.C {

			img.Refresh()
			framecount++

			fpsText.Move(fyne.NewPos(mainContainer.Size().Width-fpsText.MinSize().Width-10, 10))

			if time.Now().Sub(startTime) > time.Second {
				fps = float64(framecount) / time.Now().Sub(startTime).Seconds()
				startTime = time.Now()
				framecount = 0
			}
			fpsText.Text = "FPS: " + strconv.FormatFloat(fps, 'f', 2, 64)
			fpsText.Refresh()
		}
	}()

	go func() {
		for range ticker.C {
			chunk := spectrogram.GetAudioChunk()
			if chunk == nil {
				continue
			}

			magnitudes := spectrogram.ComputeFFT(chunk)

			currentRow := make([]color.Color, currentHeight)
			for y := 0; y < currentHeight; y++ {
				freq := float64(y) / float64(currentHeight) * 11025
				bin := int(freq * float64(spectrogram.FFTSize) / 44100)
				if bin < len(magnitudes) {
					magnitude := magnitudes[bin]
					currentRow[y] = spectrogram.MagnitudeToPixel(magnitude)
				} else {
					currentRow[y] = color.Black
				}
			}

			if historyIndex < currentWidth {
				spectrogramHistory[historyIndex] = currentRow
				historyIndex++
			} else {
				copy(spectrogramHistory, spectrogramHistory[1:])
				spectrogramHistory[currentWidth-1] = currentRow
			}
		}
	}()

	w.Resize(fyne.NewSize(800, 600))
	w.ShowAndRun()

	stream.Stop()
}
