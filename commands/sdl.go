//go:build cgo

package commands

import (
	"github.com/0magnet/calvin"

	sdlui "github.com/0magnet/audioprism-go/cgo/commands"
)

// The C implementation binds SDL2, FFTW3 and PulseAudio, so it is cgo-only.
func init() {
	RootCmd.AddCommand(sdlui.RootCmd)
	sdlui.RootCmd.Long = calvin.AsciiFont("c") + "\n" + asv + " in C with SDL2, FFTW3 and PulseAudio"
}
