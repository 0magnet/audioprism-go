// go run testing/txt/txt.go | aplay -f S16_LE -r 44100 -c 1
package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
)

// Constants for audio settings
const (
	sampleRate = 44100 // Sample rate in Hz
	duration   = 2     // Duration of the sound in seconds
)

func main() {
	// Create buffers for the two sweeps
	ascSweep := make([]float64, sampleRate*duration)
	descSweep := make([]float64, sampleRate*duration)

	// Generate ascending sweep from 160 Hz to 1600 Hz
	for i := range ascSweep {
		t := float64(i) / sampleRate
		freq := 160 + (1600-160)*(t/float64(duration))
		ascSweep[i] = math.Sin(2 * math.Pi * freq * t)
	}

	// Generate descending sweep from 1600 Hz to 160 Hz
	for i := range descSweep {
		t := float64(i) / sampleRate
		freq := 1600 - (1600-160)*(t/float64(duration))
		descSweep[i] = math.Sin(2 * math.Pi * freq * t)
	}

	// Mix the two sweeps together
	mixedSweep := make([]float64, sampleRate*duration)
	for i := range mixedSweep {
		mixedSweep[i] = 0.5 * (ascSweep[i] + descSweep[i]) // Mixing the two signals
	}

	// Open output file (stdout for piping to aplay)
	file := os.Stdout

	// Convert to 16-bit PCM data and write to stdout
	for _, sample := range mixedSweep {
		// Clamp the sample to the range of [-1, 1]
		if sample > 1 {
			sample = 1
		} else if sample < -1 {
			sample = -1
		}

		// Convert to 16-bit signed integer
		intSample := int16(sample * 32767)

		// Write the binary data to stdout
		err := binary.Write(file, binary.LittleEndian, intSample)
		if err != nil {
			fmt.Println("Error writing to output:", err)
			return
		}
	}
}
