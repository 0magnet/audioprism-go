// Package commands cmd/gomobile/commands/root.go
package commands

import (
	"github.com/spf13/cobra"

	ui "github.com/0magnet/audioprism-go/pkg/gomobile-ws"
)

var width, height int
var showFPS bool

func init() {
	RootCmd.Flags().IntVar(&width, "width", 512, "Width of the spectrogram")
	RootCmd.Flags().IntVar(&height, "height", 256, "Height of the spectrogram (typically FFTSize / 2)")
	RootCmd.Flags().BoolVar(&showFPS, "show-fps", true, "Display the FPS counter")
}

// RootCmd conta8ns the root cli command
var RootCmd = &cobra.Command{
	Use:   "mobile",
	Short: "with golang.org/x/mobile GUI",
	Long: `
	┌─┐┌─┐┌┬┐┌─┐┌┐ ┬┬  ┌─┐
	│ ┬│ │││││ │├┴┐││  ├┤
	└─┘└─┘┴ ┴└─┘└─┘┴┴─┘└─┘
	` + "Audio Spectrogram Visualization with golang.org/x/mobile GUI",
	Run: func(_ *cobra.Command, _ []string) {
		ui.Run(width, height, showFPS)
	},
}
