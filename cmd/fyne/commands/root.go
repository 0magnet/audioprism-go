// Package commands cmd/fyne/commands/root.go
package commands

import (
	"github.com/spf13/cobra"

	ui "github.com/0magnet/audioprism-go/pkg/fyne"
)

var (
	w, h, u, b int
)

func init() {
	RootCmd.Flags().IntVarP(&w, "width", "x", 800, "initial window width")
	RootCmd.Flags().IntVarP(&h, "height", "y", 600, "initial window height")
	RootCmd.Flags().IntVarP(&u, "up", "u", 60, "fps rate - 0 unlimits")
	RootCmd.Flags().IntVarP(&b, "buf", "b", 32768, "size of audio buffer")
}

// RootCmd contains the root command
var RootCmd = &cobra.Command{
	Use:   "fyne",
	Short: "with Fyne GUI",
	Long: `
	┌─┐┬ ┬┌┐┌┌─┐
	├┤ └┬┘│││├┤
	└   ┴ ┘└┘└─┘
	` + "Audio Spectrogram Visualization with Fyne GUI",
	Run: func(_ *cobra.Command, _ []string) {
		ui.Run(w, h, u, b)
	},
}
