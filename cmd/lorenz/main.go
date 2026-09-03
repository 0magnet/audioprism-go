// Command lorenz streams a Lorenz attractor as 32-bit float stereo PCM on
// stdout (48 kHz), for X-Y oscilloscope visualization — pipe it into ffmpeg.
// test2.sh drives it; keeping the generator in Go means the repo depends only
// on the Go toolchain.
package main

import (
	"bufio"
	"encoding/binary"
	"os"
)

func main() {
	const (
		sigma, rho, beta = 10.0, 28.0, 8.0 / 3.0
		dt               = 0.0001 // integration step
		sampleRate       = 48000
		duration         = 30 // seconds
		xScale, yScale   = 0.04, 0.03
		skip             = 10 // emit every Nth point, to slow the trace
	)
	samples := sampleRate * duration
	x, y, z := 1.0, 1.0, 1.0

	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()

	n := 0
	for i := 0; i < samples*skip; i++ {
		dx := sigma * (y - x)
		dy := x*(rho-z) - y
		dz := x*y - beta*z
		x += dx * dt
		y += dy * dt
		z += dz * dt
		if i%skip != 0 {
			continue
		}
		binary.Write(w, binary.LittleEndian, float32(clamp(x*xScale)))
		binary.Write(w, binary.LittleEndian, float32(clamp(y*yScale)))
		if n++; n >= samples {
			break
		}
	}
}

func clamp(v float64) float64 {
	if v < -1 {
		return -1
	}
	if v > 1 {
		return 1
	}
	return v
}
