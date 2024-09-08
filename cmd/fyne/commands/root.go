// Package commands cmd/fyne/commands/root.go
// CREATED WITH GO GENERATE DO NOT EDIT!
package commands

import (
	"github.com/spf13/cobra"

	ui "github.com/0magnet/audioprism-go/pkg/fyne"
)

var (
	w, h, u, b int
	s          bool
	k          string
)

func init() {
	RootCmd.Flags().IntVarP(&w, "width", "x", 512, "initial window width")
	RootCmd.Flags().IntVarP(&h, "height", "y", 512, "initial window height")
	RootCmd.Flags().IntVarP(&u, "up", "u", 60, "fps rate - 0 unlimits")
	RootCmd.Flags().IntVarP(&b, "buf", "b", 32768, "size of audio buffer")
	RootCmd.Flags().BoolVarP(&s, "fps", "s", false, "show fps")
	RootCmd.Flags().StringVarP(&k, "websocket", "k", "", "websocket url (i.e. 'ws://127.0.0.1:8080/ws')")
}

// RootCmd contains the root command
var RootCmd = &cobra.Command{
	SilenceErrors:         true,
	SilenceUsage:          true,
	DisableSuggestions:    true,
	DisableFlagsInUseLine: true,
	Use:                   "fyne",
	Short:                 "with fyne",
	Long: `
	┌─┐┬ ┬┌┬┐┬┌─┐┌─┐┬─┐┬┌─┐┌┬┐   ┌─┐┌─┐
	├─┤│ │ ││││ │├─┘├┬┘│└─┐│││───│ ┬│ │
	┴ ┴└─┘─┴┘┴└─┘┴  ┴└─┴└─┘┴ ┴   └─┘└─┘
	` + "Audio Spectrogram Visualization with fyne",
	Run: func(_ *cobra.Command, _ []string) {
		ui.Run(w, h, u, b, s, k)
	},
}

const help = `{{if gt (len .Aliases) 0}}{{.NameAndAliases}}{{end}}{{if .HasAvailableSubCommands}}Available Commands:{{range .Commands}}{{if (or .IsAvailableCommand)}}
{{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}
Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}
Global Flags:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}

`
