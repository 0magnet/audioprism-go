// Package spectrogram implements functions for generating spectrograms from audio data.
package spectrogram

import (
	"image/color"
	"math"
	"math/cmplx"
	"sync"

	"github.com/mjibson/go-dsp/fft"
	"github.com/mjibson/go-dsp/window"
)

const (
	// SampleRate is the rate at which audio samples are taken.
	SampleRate = 44100

	// Channels is the number of audio channels.
	Channels = 1

	// BufferSize is the size of the audio buffer.
	BufferSize = 32768

	// FFTSize is the size of the FFT window.
	FFTSize = 1024

	// OverlapRatio is the ratio of overlap between consecutive FFT windows.
	OverlapRatio = 0.5

	// MaxFrequency is the maximum frequency represented in the spectrogram.
	MaxFrequency = 12000

	// MinMagnitude is the minimum magnitude value for normalization.
	MinMagnitude = 0.0

	// MaxMagnitude is the maximum magnitude value for normalization.
	MaxMagnitude = 45.0

	// LogMagnitude determines if magnitude should be converted to logarithmic scale.
	LogMagnitude = true
)

// AudioBuffer stores the audio samples for processing.
var (
	AudioBuffer     []float32
	AudioBufferLock sync.Mutex
)

// Normalize normalizes the value between the given range.
func Normalize(value, min, max float64) float64 {
	return (math.Max(math.Min(value, max), min) - min) / (max - min)
}

// ValueToPixelHeat converts a normalized value to a color based on a heatmap.
func ValueToPixelHeat(value float64) color.Color {
	var r, g, b uint8

	if value < 1.0/5.0 {
		b = uint8(255.0 * Normalize(value, 0.0, 1.0/5.0))
	} else if value < 2.0/5.0 {
		c := uint8(255.0 * Normalize(value, 1.0/5.0, 2.0/5.0))
		r = 0
		g = c
		b = 255 - c
	} else if value < 3.0/5.0 {
		r = uint8(255.0 * Normalize(value, 2.0/5.0, 3.0/5.0))
		g = 255
		b = 0
	} else if value < 4.0/5.0 {
		r = 255
		g = uint8(255 - 255.0*Normalize(value, 3.0/5.0, 4.0/5.0))
		b = 0
	} else {
		c := uint8(255.0 * Normalize(value, 4.0/5.0, 1.0)) // Changed 5.0/5.0 to 1.0
		r = 255
		g = c
		b = c
	}

	return color.RGBA{r, g, b, 255}
}

// ValueToPixelBlue converts a normalized value to a color based on a blue gradient.
func ValueToPixelBlue(value float64) color.Color {
	var r, g, b uint8

	if value < 0.5 {
		b = uint8(255.0 * Normalize(value, 0.0, 0.5))
	} else {
		c := uint8(255.0 * Normalize(value, 0.5, 1.0))
		r = c
		g = c
		b = 255
	}

	return color.RGBA{r, g, b, 255}
}

// ValueToPixelGrayscale converts a normalized value to a grayscale color.
func ValueToPixelGrayscale(value float64) color.Color {
	c := uint8(255.0 * value)
	return color.RGBA{c, c, c, 255}
}

// MagnitudeToPixel converts a magnitude value to a pixel color.
func MagnitudeToPixel(value float64) color.Color {
	if LogMagnitude {
		value = 20 * math.Log10(value+1e-10)
	}
	return ValueToPixelHeat(Normalize(value, MinMagnitude, MaxMagnitude))
}

// ComputeFFT computes the FFT of the input and returns the magnitudes.
func ComputeFFT(input []float32) []float64 {
	hammingWindow := window.Hamming(FFTSize)
	windowedBuffer := make([]float64, FFTSize)
	for i := 0; i < FFTSize; i++ {
		windowedBuffer[i] = float64(input[i]) * hammingWindow[i]
	}

	spectrum := fft.FFTReal(windowedBuffer)
	magnitudes := make([]float64, len(spectrum)/2)
	for i := 0; i < len(magnitudes); i++ {
		magnitudes[i] = cmplx.Abs(spectrum[i])
	}
	return magnitudes
}

// GetAudioChunk retrieves a chunk of the audio buffer for FFT processing.
func GetAudioChunk() []float32 {
	AudioBufferLock.Lock()
	defer AudioBufferLock.Unlock()

	if len(AudioBuffer) < FFTSize {
		return nil
	}

	chunk := AudioBuffer[:FFTSize]
	stepSize := int(float64(FFTSize) * (1 - OverlapRatio))
	AudioBuffer = AudioBuffer[stepSize:]
	return chunk
}
