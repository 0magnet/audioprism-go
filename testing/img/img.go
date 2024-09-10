// Package main testing image encoding as frequencies
// go run testing/img/img.go | aplay -f S16_LE -r 44100 -c 1
package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
)

const (
	sampleRate = 44100
	duration   = 30
	amplitude  = 0.5
	quiet      = 0
)

func main() {
	imgData, err := os.ReadFile("letter.png")
	if err != nil {
		log.Fatal("Error reading image file:", err)
	}

	img, err := png.Decode(bytes.NewReader(imgData))
	if err != nil {
		log.Fatal("Error decoding image:", err)
	}
	log.Println("decoded image")

	width := img.Bounds().Dx()
	height := img.Bounds().Dy()
	log.Printf("Image dimensions: width=%d, height=%d", width, height)

	segmentDuration := duration / float64(width)
	segmentSampleCount := int(sampleRate * segmentDuration)

	audioData := make([]float64, sampleRate*duration)
	log.Printf("created buffer make([]float64,%v) \n", sampleRate*duration)
	pstep := (height - 1) / 8
	fstep := 180
	p := 0
	f := 1600
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

	processRow(img, height-1, 160, segmentSampleCount, audioData)

	for _, sample := range audioData {
		if sample > 0.1 {
			sample = 0.1
		} else if sample < -0.1 {
			sample = -0.1
		}

		intSample := int16(sample * 32767)

		err := binary.Write(os.Stdout, binary.LittleEndian, intSample)
		if err != nil {
			log.Fatal("Error writing to output:", err)
		}
	}
}

func processRow(img image.Image, row int, baseFrequency float64, segmentSampleCount int, audioData []float64) {
	width := img.Bounds().Dx()
	for x := 0; x < width; x++ {
		start := x * segmentSampleCount
		end := start + segmentSampleCount

		pixel := color.GrayModel.Convert(img.At(x, row)).(color.Gray)
		if pixel.Y < 128 {
			freq := baseFrequency

			for i := start; i < end; i++ {
				time := float64(i) / sampleRate
				audioData[i] += amplitude * math.Sin(2*math.Pi*freq*time)
			}
		} // else {
		//			for i := start; i < end; i++ {
		// No need to set to quiet explicitly since it's already initialized to quiet
		//			}
		//	}
	}
}
