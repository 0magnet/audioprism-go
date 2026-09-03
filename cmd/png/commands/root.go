// Package commands cmd/png/commands/root.go
//
// The file mode: a WAV in, an image out, nothing live involved.
//
// The original audioprism has had this alongside its real-time display from the
// start — "WAV File Usage: audioprism [options] <WAV file input> <image file
// output>" — and this port did not. It is worth more than the convenience of
// rendering a recording. It is the only mode in which either program can be
// checked: the same file rendered twice gives the same pixels, so a port can be
// held against the thing it is a port of and the difference measured. Every
// claim in pkg/spectrogram about matching the C++ was settled this way, and
// could not have been settled without it.
package commands

import (
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	sg "github.com/0magnet/audioprism-go/pkg/spectrogram"
	"github.com/0magnet/audioprism-go/pkg/wav"
)

var (
	width       int
	height      int
	orientation string
	colors      string
	winFunc     string
	magScale    string
	magMin      float64
	magMax      float64
	dftSize     int
	overlap     float64
	quality     int
)

func init() {
	f := RootCmd.Flags()
	f.IntVarP(&width, "width", "x", 640, "width of spectrogram")
	f.IntVarP(&height, "height", "y", 480, "height of spectrogram")
	f.StringVar(&orientation, "orientation", "vertical", "orientation: horizontal, vertical")
	f.StringVar(&colors, "colors", "heat", "color scheme: heat, blue, grayscale, turbo, viridis, magma")
	f.StringVar(&winFunc, "window", "hann", "window function: hann, hamming, bartlett, rectangular")
	f.StringVar(&magScale, "magnitude-scale", "logarithmic", "magnitude scale: logarithmic, linear")
	f.Float64Var(&magMin, "magnitude-min", 0.0, "magnitude minimum")
	f.Float64Var(&magMax, "magnitude-max", 45.0, "magnitude maximum")
	f.IntVar(&dftSize, "dft-size", 1024, "DFT size (power of 2, 64-8192)")
	f.Float64Var(&overlap, "overlap", 0.50, "samples overlap percentage (5-95), or a ratio (0.05-0.95)")
	f.IntVar(&quality, "quality", 95, "JPEG quality, when the output is a .jpg")
}

// RootCmd contains the root command
var RootCmd = &cobra.Command{
	SilenceErrors:         true,
	SilenceUsage:          true,
	DisableSuggestions:    true,
	DisableFlagsInUseLine: true,
	Use:                   "png <WAV file input> <image file output>",
	Short:                 "render a WAV file to a spectrogram image",
	Args:                  cobra.ExactArgs(2),
	RunE: func(_ *cobra.Command, args []string) error {
		orient, err := parseOrientation(orientation)
		if err != nil {
			return err
		}

		// A fresh Settings rather than the package-level one: this command
		// renders exactly what it was asked for and leaves no state behind for
		// anything sharing the process.
		s := sg.DefaultSettings()
		s.SetColorByName(colors)
		s.SetWindowByName(winFunc)
		s.SetScaleByName(magScale)
		// Max first, so a pair given together is not rejected against a floor
		// that is about to move.
		s.SetMagMax(magMax)
		s.SetMagMin(magMin)
		s.SetDFTSize(dftSize)
		s.SetOverlap(sg.OverlapFromFlag(overlap))

		audio, err := wav.ReadFile(args[0])
		if err != nil {
			return fmt.Errorf("reading %s: %w", args[0], err)
		}

		// The spectrum occupies the axis it is drawn along: the image's width
		// when frequency runs across it, its height when frequency runs up it.
		spectrumWidth := width
		if orient == sg.Horizontal {
			spectrumWidth = height
		}

		img := sg.Render(audio.Samples, spectrumWidth, orient, s)
		if img == nil {
			return fmt.Errorf("%s holds %d samples, fewer than the %d one DFT frame needs",
				args[0], len(audio.Samples), s.GetDFTSize())
		}

		if err := writeImage(args[1], img, quality); err != nil {
			return fmt.Errorf("writing %s: %w", args[1], err)
		}

		b := img.Bounds()
		fmt.Printf("%s → %s: %dx%d, %d Hz, %d-point %s window, %d-sample hop, %s %g..%g, %s\n",
			args[0], args[1], b.Dx(), b.Dy(), audio.SampleRate,
			s.GetDFTSize(), s.WindowName(), s.StepSize(),
			s.ScaleName(), magMin, magMax, s.ColorName())
		return nil
	},
}

func parseOrientation(name string) (sg.Orientation, error) {
	switch strings.ToLower(name) {
	case "vertical", "v":
		return sg.Vertical, nil
	case "horizontal", "h":
		return sg.Horizontal, nil
	}
	return sg.Vertical, fmt.Errorf("unknown orientation %q (want horizontal or vertical)", name)
}

// writeImage picks the encoder from the output's extension, the way the
// original leaves the choice to the filename it is handed.
func writeImage(path string, img *image.RGBA, quality int) error {
	f, err := os.Create(path) //nolint:gosec // the path is the file the user asked to render
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck

	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg":
		if err := jpeg.Encode(f, img, &jpeg.Options{Quality: quality}); err != nil {
			return err
		}
	default:
		if err := png.Encode(f, img); err != nil {
			return err
		}
	}
	return f.Close()
}
