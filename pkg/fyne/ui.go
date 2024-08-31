package fyneui

import (
	"image"
	"image/color"
	"image/draw"
	"log"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"github.com/jfreymuth/pulse"

	"github.com/0magnet/audioprism-go/pkg/spectrogram"
)

// Run initializes and starts the Fyne application
func Run(width, height, _, _ int) {
	a := app.New()
	w := a.NewWindow("audioprism-go")
	spectrogramHistory := make([][]color.Color, width)
	historyIndex := 0
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

	c, err := pulse.NewClient()
	if err != nil {
		log.Fatal(err.Error())
	}
	defer c.Close()
	stream, err := c.NewRecord(pulse.Float32Writer(func(p []float32) (int, error) {
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
	}), pulse.RecordLatency(0.1))
	if err != nil {
		log.Fatal(err.Error())
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
		fpsText.Move(fyne.NewPos(mainContainer.Size().Width-fpsText.MinSize().Width-10, 10))
		for range ticker.C {
			framecount++
			img.Refresh()
			if time.Now().Sub(startTime) > 2*time.Second {
				fps = float64(framecount) / time.Now().Sub(startTime).Seconds()
				startTime = time.Now()
				framecount = 0
				fpsText.Text = "FPS: " + strconv.FormatFloat(fps, 'f', 2, 64)
				fpsText.Refresh()
			}
		}
	}()

	w.Resize(fyne.NewSize(800, 600))
	stream.Start()
	w.ShowAndRun()

	stream.Stop()
}
