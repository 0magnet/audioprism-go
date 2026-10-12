// Package commands testing/txt/commands/root.go
// go run . testtxt | aplay -f S16_LE -r 44100 -c 1
package commands

import (
	"github.com/spf13/cobra"

	"encoding/binary"
	"fmt"
	"math"
	"os"
)

const (
	sampleRate = 44100
	duration   = 2
)

func run() {
	ascSweep := make([]float64, sampleRate*duration)
	descSweep := make([]float64, sampleRate*duration)

	for i := range ascSweep {
		t := float64(i) / sampleRate
		freq := 160 + (1600-160)*(t/float64(duration))
		ascSweep[i] = math.Sin(2 * math.Pi * freq * t)
	}

	for i := range descSweep {
		t := float64(i) / sampleRate
		freq := 1600 - (1600-160)*(t/float64(duration))
		descSweep[i] = math.Sin(2 * math.Pi * freq * t)
	}

	mixedSweep := make([]float64, sampleRate*duration)
	for i := range mixedSweep {
		mixedSweep[i] = 0.5 * (ascSweep[i] + descSweep[i])
	}

	file := os.Stdout

	for _, sample := range mixedSweep {
		if sample > 1 {
			sample = 1
		} else if sample < -1 {
			sample = -1
		}

		intSample := int16(sample * 32767)

		err := binary.Write(file, binary.LittleEndian, intSample)
		if err != nil {
			fmt.Println("Error writing to output:", err)
			return
		}
	}
}

// RootCmd is the testtxt subcommand.
var RootCmd = &cobra.Command{
	Use:                   "testtxt",
	Short:                 "sweep tones as 16-bit PCM on stdout",
	DisableFlagsInUseLine: true,
	Args:                  cobra.NoArgs,
	Run:                   func(_ *cobra.Command, _ []string) { run() },
}
