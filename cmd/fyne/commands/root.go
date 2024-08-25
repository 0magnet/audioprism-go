// Package commands cmd/fyne/commands/root.go
package commands

import (
	"github.com/spf13/cobra"

	ui "github.com/0magnet/audioprism-go/pkg/fyne"
)

var updateRate int

func init() {
	RootCmd.Flags().IntVarP(&updateRate, "up", "u", 60, "Update rate")
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
		ui.Run(updateRate)
	},
}
