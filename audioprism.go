// Package main audioprism.go
//
//go:generate go run . gen -w -p cmd/a
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/0magnet/audioprism-go/internal/flags"
	"github.com/0magnet/calvin"
	"github.com/spf13/cobra"

	fyneui "github.com/0magnet/audioprism-go/cmd/a/fyne/commands"
	tcellui "github.com/0magnet/audioprism-go/cmd/a/tcell/commands"
	wasm "github.com/0magnet/audioprism-go/cmd/a/wasm/commands"
	pngui "github.com/0magnet/audioprism-go/cmd/png/commands"
	xyui "github.com/0magnet/audioprism-go/cmd/xy/commands"
)

const asv = "Audio Spectrogram Visualization"

func init() {
	flags.InitFlags(RootCmd, true)
	RootCmd.AddCommand(
		fyneui.RootCmd,
		wasm.RootCmd,
		tcellui.RootCmd,
		xyui.RootCmd,
		pngui.RootCmd,
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

var use = strings.Split(filepath.Base(strings.ReplaceAll(strings.ReplaceAll(fmt.Sprintf("%v", os.Args), "[", ""), "]", "")), " ")[0]
var long = use + "\n" + calvin.AsciiFont("audioprism-go") + "\n" + asv

// RootCmd contains the root command
var RootCmd = &cobra.Command{
	SilenceErrors:         true,
	SilenceUsage:          true,
	DisableSuggestions:    true,
	DisableFlagsInUseLine: true,
	Use:                   use,
	Short:                 "Audio Spectrogram Visualization",
	Long:                  long,
}

func main() {
	if err := RootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
