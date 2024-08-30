// Package commands cmd/gomobile/commands/root.go
package commands

import (
	"github.com/spf13/cobra"

	ui "github.com/0magnet/audioprism-go/pkg/gomobile"
)

var (
	width, height, bufferSize int
	showFPS                   bool
)

func init() {
	RootCmd.Flags().IntVar(&width, "width", 512, "width of the spectrogram")
	RootCmd.Flags().IntVar(&height, "height", 256, "height of the spectrogram")
	RootCmd.Flags().BoolVar(&showFPS, "show-fps", false, "show frames per second counter")
	RootCmd.Flags().IntVarP(&bufferSize, "buf", "b", 32768, "size of audio buffer")

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
		ui.Run(width, height, bufferSize, showFPS)
	},
}
