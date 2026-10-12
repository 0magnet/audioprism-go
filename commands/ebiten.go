package commands

import (
	"github.com/0magnet/calvin"

	ebitenui "github.com/0magnet/audioprism-go/xy/commands"
)

// The Ebiten X-Y scope, beside the Fyne one registered as "xy".
func init() {
	RootCmd.AddCommand(ebitenui.RootCmd)
	ebitenui.RootCmd.Use = "xy-ebiten"
	ebitenui.RootCmd.Long = calvin.AsciiFont("xy-ebiten") + "\n" + "X-Y Audio Oscilloscope with Ebiten"
}
