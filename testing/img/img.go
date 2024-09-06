package main

import (
	"bytes"
	"encoding/binary"
	"image/color"
	"image"
	"image/png"
	"log"
	"math"
	"os"
)

// Constants for audio settings
const (
	sampleRate = 44100 // Sample rate in Hz
	duration   = 20     // Duration of the sound in seconds
	amplitude  = 0.5   // Amplitude of the sine waves
	quiet      = 0     // Silence level
)

func main() {
	// Read the image file into a byte slice
	imgData, err := os.ReadFile("letter.png")
	if err != nil {
		log.Fatal("Error reading image file:", err)
	}

	// Decode the image
	img, err := png.Decode(bytes.NewReader(imgData))
	if err != nil {
		log.Fatal("Error decoding image:", err)
	}
	log.Println("decoded image")

	// Get image dimensions
	width := img.Bounds().Dx()
	height := img.Bounds().Dy()
	log.Printf("Image dimensions: width=%d, height=%d", width, height)

	// Calculate the duration of each pixel segment
	segmentDuration := duration / float64(width)
	segmentSampleCount := int(sampleRate * segmentDuration)

	// Create a buffer to hold the generated audio data
	audioData := make([]float64, sampleRate*duration)
	log.Printf("created buffer make([]float64,%v) \n", sampleRate*duration)
	pstep := (height-1)/8
	fstep := 180
	p:=0
	f:=1600
	// Process the top row
//	processRow(img, 0, 1600, segmentSampleCount, audioData)
	processRow(img, p, float64(f), segmentSampleCount, audioData)
	p += pstep
	f -= fstep
	processRow(img, p, float64(f), segmentSampleCount, audioData)
	p += pstep
	f -= fstep
	processRow(img, p, float64(f), segmentSampleCount, audioData)
	p += pstep
	f -= fstep
	processRow(img, p, float64(f), segmentSampleCount, audioData)
	p += pstep
	f -= fstep
	processRow(img, p, float64(f), segmentSampleCount, audioData)
	p += pstep
	f -= fstep
	processRow(img, p, float64(f), segmentSampleCount, audioData)
	p += pstep
	f -= fstep
	processRow(img, p, float64(f), segmentSampleCount, audioData)
	p += pstep
	f -= fstep
	processRow(img, p, float64(f), segmentSampleCount, audioData)
//	processRow(img, 0, 1600, segmentSampleCount, audioData)

	// Process the bottom row
	processRow(img, height-1, 160, segmentSampleCount, audioData)

	// Convert to 16-bit PCM data and write to stdout
	for _, sample := range audioData {
		// Clamp the sample to the range of [-1, 1]
		if sample > 0.1 {
			sample = 0.1
		} else if sample < -0.1 {
			sample = -0.1
		}

		// Convert to 16-bit signed integer
		intSample := int16(sample * 32767)

		// Write the binary data to stdout
		err := binary.Write(os.Stdout, binary.LittleEndian, intSample)
		if err != nil {
			log.Fatal("Error writing to output:", err)
		}
	}
}

// processRow generates audio data for a given row
func processRow(img image.Image, row int, baseFrequency float64, segmentSampleCount int, audioData []float64) {
	width := img.Bounds().Dx()
	for x := 0; x < width; x++ {
		// Determine the start and end indices for this pixel's segment
		start := x * segmentSampleCount
		end := start + segmentSampleCount

		// Check if the pixel is black
		pixel := color.GrayModel.Convert(img.At(x, row)).(color.Gray)
		if pixel.Y < 128 {
			// Frequency for black pixels
			freq := baseFrequency

			// Generate sine wave for the current time slice
			for i := start; i < end; i++ {
				time := float64(i) / sampleRate
				audioData[i] += amplitude * math.Sin(2*math.Pi*freq*time)
			}
		} else {
			// Silence for white pixels
			for i := start; i < end; i++ {
				// No need to set to quiet explicitly since it's already initialized to quiet
			}
		}
	}
}
