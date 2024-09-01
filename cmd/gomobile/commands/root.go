// Package commands cmd/gomobile/commands/root.go
package commands

import (
	"github.com/spf13/cobra"

	ui "github.com/0magnet/audioprism-go/pkg/gomobile"
)

var (
	w, h, u, b int
	s          bool
	k          string
)

func init() {
	RootCmd.Flags().IntVarP(&w, "width", "x", 512, "initial window width")
	RootCmd.Flags().IntVarP(&h, "height", "y", 256, "initial window height")
	RootCmd.Flags().IntVarP(&u, "up", "u", 60, "fps rate - 0 unlimits")
	RootCmd.Flags().IntVarP(&b, "buf", "b", 32768, "size of audio buffer")
	RootCmd.Flags().BoolVarP(&s, "fps", "s", false, "show fps")
	RootCmd.Flags().StringVarP(&k, "websocket", "k", "", "websocket url (i.e. 'ws://127.0.0.1:8080/ws')")

}

// RootCmd conta8ns the root cli command
var RootCmd = &cobra.Command{
	Use:   "mobile",
	Short: "with golang.org/x/mobile GUI",
	Long: `
	┌─┐┬ ┬┌┬┐┬┌─┐┌─┐┬─┐┬┌─┐┌┬┐   ┌─┐┌─┐
	├─┤│ │ ││││ │├─┘├┬┘│└─┐│││───│ ┬│ │
	┴ ┴└─┘─┴┘┴└─┘┴  ┴└─┴└─┘┴ ┴   └─┘└─┘
	` + "Audio Spectrogram Visualization with golang.org/x/mobile GUI",
	Run: func(_ *cobra.Command, _ []string) {
		ui.Run(w, h, u, b, s, k)
	},
}
