//go:build cgo

// Package commands /gomobile/commands/root.go
// CREATED WITH GO GENERATE DO NOT EDIT!
package commands

import (
	"github.com/spf13/cobra"

	sg "github.com/0magnet/audioprism-go/pkg/spectrogram"
	ui "github.com/0magnet/audioprism-go/pkg/ui/gomobile"
)

var (
	w, h, u, b int
	s          bool
	k          string
	colors     string
	winFunc    string
	magScale   string
	magMin     float64
	magMax     float64
	dftSize    int
	overlap    float64
)

func init() {
	RootCmd.Flags().IntVarP(&w, "width", "x", 640, "initial window width")
	RootCmd.Flags().IntVarP(&h, "height", "y", 480, "initial window height")
	RootCmd.Flags().IntVarP(&u, "up", "u", 60, "fps rate - 0 unlimits")
	RootCmd.Flags().IntVarP(&b, "buf", "b", 32768, "size of audio buffer")
	RootCmd.Flags().BoolVarP(&s, "fps", "s", false, "show fps")
	RootCmd.Flags().StringVarP(&k, "websocket", "k", "", "websocket url (i.e. 'ws://127.0.0.1:8080/ws')")
	RootCmd.Flags().StringVar(&colors, "colors", "heat", "color scheme: heat, blue, grayscale, turbo, viridis, magma")
	RootCmd.Flags().StringVar(&winFunc, "window", "hann", "window function: hann, hamming, bartlett, rectangular")
	RootCmd.Flags().StringVar(&magScale, "magnitude-scale", "log", "magnitude scale: log, linear")
	RootCmd.Flags().Float64Var(&magMin, "magnitude-min", 0.0, "magnitude minimum")
	RootCmd.Flags().Float64Var(&magMax, "magnitude-max", 45.0, "magnitude maximum")
	RootCmd.Flags().IntVar(&dftSize, "dft-size", 1024, "DFT size (power of 2, 64-8192)")
	RootCmd.Flags().Float64Var(&overlap, "overlap", 0.50, "samples overlap percentage (5-95), or a ratio (0.05-0.95)")
}

// RootCmd contains the root command
var RootCmd = &cobra.Command{
	SilenceErrors:         true,
	SilenceUsage:          true,
	DisableSuggestions:    true,
	DisableFlagsInUseLine: true,
	Use:                   "gomobile",
	Short:                 "with gomobile",
	Long: `┌─┐┌─┐┌┬┐┌─┐┌┐ ┬┬  ┌─┐
│ ┬│ │││││ │├┴┐││  ├┤ 
└─┘└─┘┴ ┴└─┘└─┘┴┴─┘└─┘` + "\nAudio Spectrogram Visualization with gomobile",
	Run: func(_ *cobra.Command, _ []string) {
		sg.S.SetColorByName(colors)
		sg.S.SetWindowByName(winFunc)
		sg.S.SetScaleByName(magScale)
		sg.S.SetMagMin(magMin)
		sg.S.SetMagMax(magMax)
		sg.S.SetDFTSize(dftSize)
		sg.S.SetOverlap(sg.OverlapFromFlag(overlap))
		ui.Run(w, h, u, b, s, k)
	},
}
