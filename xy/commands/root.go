// Package commands xy/commands/root.go
package commands

import (
	"fmt"
	"image/color"
	"log"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
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

type Game struct {
	points    []Point
	scale     float32
	lastLeft  float32
	lastRight float32
	plotCount int
}

func (g *Game) Update() error {
	// Keyboard controls for scale
	if inpututil.IsKeyJustPressed(ebiten.KeyEqual) || inpututil.IsKeyJustPressed(ebiten.KeyKPAdd) {
		g.scale += 50
		log.Printf("Scale: %.0f", g.scale)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyMinus) || inpututil.IsKeyJustPressed(ebiten.KeyKPSubtract) {
		g.scale -= 50
		if g.scale < 50 {
			g.scale = 50
		}
		log.Printf("Scale: %.0f", g.scale)
	}
	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	// Clear screen
	screen.Fill(color.RGBA{0, 0, 0, 255})

	centerX := float32(screenWidth / 2)
	centerY := float32(screenHeight / 2)

	// Draw grid lines
	for i := 0; i < screenWidth; i++ {
		screen.Set(i, screenHeight/2, color.RGBA{30, 30, 30, 255})
	}
	for i := 0; i < screenHeight; i++ {
		screen.Set(screenWidth/2, i, color.RGBA{30, 30, 30, 255})
	}

	// Draw points with fade effect
	numPoints := len(g.points)
	for i, p := range g.points {
		x := int(centerX + p.X*g.scale)
		y := int(centerY - p.Y*g.scale)

		if x < 0 || x >= screenWidth || y < 0 || y >= screenHeight {
			continue
		}

		// Fade effect
		age := float32(i) / float32(numPoints)
		brightness := uint8(age * 255)
		c := color.RGBA{0, brightness, 0, 255}

		// Draw point with thickness
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				px := x + dx
				py := y + dy
				if px >= 0 && px < screenWidth && py >= 0 && py < screenHeight {
					screen.Set(px, py, c)
				}
			}
		}
	}

	// Draw info text
	info := fmt.Sprintf("Scale: %.0f | Points: %d | L: %+.3f R: %+.3f | +/- to zoom",
		g.scale, numPoints, g.lastLeft, g.lastRight)
	ebitenutil.DebugPrint(screen, info)
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return screenWidth, screenHeight
}

func (g *Game) AddPoint(x, y float32) {
	g.lastLeft = x
	g.lastRight = y
	g.plotCount++

	// Log every 100th point with actual coordinates
	if printLogs && g.plotCount%100 == 0 {
		centerX := float32(screenWidth / 2)
		centerY := float32(screenHeight / 2)
		screenX := int(centerX + x*g.scale)
		screenY := int(centerY - y*g.scale)
		log.Printf("Plot %d: X=%.3f Y=%.3f ScreenX=%d ScreenY=%d Scale=%.0f",
			g.plotCount, x, y, screenX, screenY, g.scale)
	}

	g.points = append(g.points, Point{X: x, Y: y})
	if len(g.points) > maxPoints {
		g.points = g.points[1:]
	}
}

var logFile string
var printLogs bool

func init() {
	RootCmd.Flags().StringVarP(&logFile, "file", "f", "", "log to file if specified")
	RootCmd.Flags().BoolVarP(&printLogs, "log", "l", false, "enable print plot point logging")
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
` + "\nX-Y Audio scope with hajimehoshi/ebiten v2",
	Run: func(_ *cobra.Command, _ []string) {
		// Set up file logging if requested
		if logFile != "" {
			f, err := os.Create(logFile) //nolint:gosec // the path is the log file named on the command line
			if err != nil {
				log.Fatal("Failed to create log file:", err)
			}
			defer f.Close() //nolint:errcheck // the log file is closed as the program exits; the log has nowhere left to go
			log.SetOutput(f)
		}
		log.Println("XY Scope started")

		game := &Game{
			points: make([]Point, 0, maxPoints),
			scale:  300.0,
		}

		// Connect to PulseAudio
		c, err := pulse.NewClient()
		if err != nil {
			log.Fatal("Failed to connect to PulseAudio:", err)
		}
		defer c.Close()

		// Callback to process audio samples
		sampleCount := 0
		processAudio := func(samples []float32) (int, error) {
			// Log first 100 sample batches if logging enabled
			if printLogs && logFile != "" && sampleCount < 100 {
				if len(samples) >= 20 {
					log.Printf("Batch %d: L[0-9]=%.3f,%.3f,%.3f,%.3f,%.3f,%.3f,%.3f,%.3f,%.3f,%.3f R[0-9]=%.3f,%.3f,%.3f,%.3f,%.3f,%.3f,%.3f,%.3f,%.3f,%.3f",
						sampleCount,
						samples[0], samples[2], samples[4], samples[6], samples[8],
						samples[10], samples[12], samples[14], samples[16], samples[18],
						samples[1], samples[3], samples[5], samples[7], samples[9],
						samples[11], samples[13], samples[15], samples[17], samples[19])
				}
			}
			sampleCount++

			// Process stereo samples (interleaved: L, R, L, R, ...)
			for i := 0; i < len(samples)-1; i += 2 {
				left := samples[i]
				right := samples[i+1]
				game.AddPoint(left, right)
			}
			return len(samples), nil
		}

		// Create a record stream with explicit stereo channel map
		stream, err := c.NewRecord(
			pulse.Float32Writer(processAudio),
			pulse.RecordSampleRate(sampleRate),
			pulse.RecordChannels(proto.ChannelMap{proto.ChannelLeft, proto.ChannelRight}),
			pulse.RecordLatency(0.05), // Lower latency for responsiveness
		)
		if err != nil {
			log.Fatal("Failed to create record stream:", err)
		}
		defer stream.Close()

		stream.Start()

		// Run the game
		ebiten.SetWindowSize(screenWidth, screenHeight)
		ebiten.SetWindowTitle("X-Y Oscilloscope")
		ebiten.SetTPS(60)

		if err := ebiten.RunGame(game); err != nil {
			log.Fatal(err)
		}
	},
}
