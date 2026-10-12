// Package commands is the audioprism command tree: every native tool in this
// repository is a subcommand of RootCmd.
package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/0magnet/calvin"
	"github.com/0magnet/calvin/clihelp"

	fyneui "github.com/0magnet/audioprism-go/cmd/a/fyne/commands"
	tcellui "github.com/0magnet/audioprism-go/cmd/a/tcell/commands"
	wasm "github.com/0magnet/audioprism-go/cmd/a/wasm/commands"
	lorenz "github.com/0magnet/audioprism-go/cmd/lorenz/commands"
	pngui "github.com/0magnet/audioprism-go/cmd/png/commands"
	xyui "github.com/0magnet/audioprism-go/cmd/xy/commands"
	testimg "github.com/0magnet/audioprism-go/testing/img/commands"
	testtxt "github.com/0magnet/audioprism-go/testing/txt/commands"
)

const asv = "Audio Spectrogram Visualization"

// RootCmd contains the root command
var RootCmd = &cobra.Command{
	SilenceErrors:         true,
	SilenceUsage:          true,
	DisableSuggestions:    true,
	DisableFlagsInUseLine: true,
	Use:                   "audioprism",
	Short:                 asv,
	Long:                  asv,
}

func init() {
	RootCmd.AddCommand(
		fyneui.RootCmd,
		wasm.RootCmd,
		tcellui.RootCmd,
		xyui.RootCmd,
		pngui.RootCmd,
		lorenz.RootCmd,
		testimg.RootCmd,
		testtxt.RootCmd,
	)

	fyneui.RootCmd.Use = "f"
	fyneui.RootCmd.Long = calvin.AsciiFont("fyne") + "\n" + asv + " with Fyne GUI"
	wasm.RootCmd.Use = "w"
	wasm.RootCmd.Long = calvin.AsciiFont("wasm") + "\n" + asv + " in Webassembly"
	tcellui.RootCmd.Use = "t"
	tcellui.RootCmd.Long = calvin.AsciiFont("tcell") + "\n" + asv + " with Tcell TUI"
	pngui.RootCmd.Use = "p"
	pngui.RootCmd.Long = calvin.AsciiFont("png") + "\n" + asv + " from a WAV file to an image file"
	xyui.RootCmd.Use = "xy"
	xyui.RootCmd.Long = calvin.AsciiFont("xy") + "\n" + "X-Y Audio Oscilloscope"
	RootCmd.CompletionOptions.DisableDefaultCmd = true
	wasm.RootCmd.CompletionOptions.DisableDefaultCmd = true
}

// Execute runs the command tree. The house help is installed here, once the
// subcommand files (some of them cgo-only) have all attached themselves.
func Execute() {
	clihelp.Init(RootCmd, "audioprism")
	if err := RootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
