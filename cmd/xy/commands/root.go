// Package commands cmd/xy/commands/root.go
package commands

import (
	"image/color"
	"log"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"github.com/jfreymuth/pulse"
	"github.com/jfreymuth/pulse/proto"
	"github.com/spf13/cobra"
)

const (
	screenWidth  = 800
	screenHeight = 800
	sampleRate   = 48000
	maxPoints    = 3000
)

type Point struct {
	X, Y float32
}

type XYScope struct {
	points    []Point
	scale     float32
	canvas    *fyne.Container
	raster    *canvas.Raster
	lastLeft  float32
	lastRight float32
}

func NewXYScope() *XYScope {
	scope := &XYScope{
		points: make([]Point, 0, maxPoints),
		scale:  300.0,
	}

	scope.raster = canvas.NewRasterWithPixels(scope.draw)
	scope.canvas = container.NewStack(scope.raster)

	return scope
}

func (s *XYScope) draw(x, y, w, h int) color.Color {
	// Convert screen coordinates to normalized [-1, 1]
	centerX := float32(w / 2)
	centerY := float32(h / 2)

	// Background
	bgColor := color.RGBA{0, 0, 0, 255}
	gridColor := color.RGBA{30, 30, 30, 255}

	// Draw grid lines
	if x == w/2 || y == h/2 {
		return gridColor
	}

	// Draw points with fade effect
	numPoints := len(s.points)
	if numPoints == 0 {
		return bgColor
	}

	// Check if any point is near this pixel
	for i, p := range s.points {
		px := int(centerX + p.X*s.scale)
		py := int(centerY - p.Y*s.scale)

		// Check if point is within 2 pixels of current position
		dx := px - x
		dy := py - y
		if dx*dx+dy*dy <= 4 {
			// Fade effect
			age := float32(i) / float32(numPoints)
			brightness := uint8(age * 255)
			return color.RGBA{0, brightness, 0, 255}
		}
	}

	return bgColor
}

func (s *XYScope) AddPoint(x, y float32) {
	s.lastLeft = x
	s.lastRight = y
	s.points = append(s.points, Point{X: x, Y: y})
	if len(s.points) > maxPoints {
		s.points = s.points[1:]
	}
	// UI updates must happen on Fyne's main thread
	fyne.Do(func() {
		s.raster.Refresh()
	})
}

// RootCmd contains the root command
var RootCmd = &cobra.Command{
	SilenceErrors:         true,
	SilenceUsage:          true,
	DisableSuggestions:    true,
	DisableFlagsInUseLine: true,
	Use:                   "xy",
	Short:                 "X-Y Audio scope",
	Long: `
─┐ ┬┬ ┬
┌┴┬┘└┬┘
┴ └─ ┴
` + "\nX-Y Audio scope with Fyne GUI",
	Run: func(_ *cobra.Command, _ []string) {
		log.Println("XY Scope started")

		myApp := app.New()
		myWindow := myApp.NewWindow("X-Y Oscilloscope")

		scope := NewXYScope()

		// Connect to PulseAudio
		c, err := pulse.NewClient()
		if err != nil {
			log.Fatal("Failed to connect to PulseAudio:", err)
		}
		defer c.Close()

		// Callback to process audio samples
		processAudio := func(samples []float32) (int, error) {
			// Process stereo samples (interleaved: L, R, L, R, ...)
			for i := 0; i < len(samples)-1; i += 2 {
				left := samples[i]
				right := samples[i+1]
				scope.AddPoint(left, right)
			}
			return len(samples), nil
		}

		// Create a record stream with explicit stereo channel map
		stream, err := c.NewRecord(
			pulse.Float32Writer(processAudio),
			pulse.RecordSampleRate(sampleRate),
			pulse.RecordChannels(proto.ChannelMap{proto.ChannelLeft, proto.ChannelRight}),
			pulse.RecordLatency(0.05),
		)
		if err != nil {
			log.Fatal("Failed to create record stream:", err)
		}
		defer stream.Close()

		stream.Start()

		myWindow.SetContent(scope.canvas)
		myWindow.Resize(fyne.NewSize(screenWidth, screenHeight))
		myWindow.ShowAndRun()
	},
}
