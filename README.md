[![Go Report Card](https://goreportcard.com/badge/github.com/0magnet/audioprism-go)](https://goreportcard.com/report/github.com/0magnet/audioprism-go)

# audioprism-go

**Work In Progress**

Port of [audioprism](https://github.com/vsergeev/audioprism) to golang

## Frontends

 * [Fyne](https://github.com/fyne-io/fyne)
* [Go mobile](https://pkg.go.dev/golang.org/x/mobile)
* Web Assembly WASM

## Audio Source

Support for **pulseaudio** via  "[github.com/jfreymuth/pulse](https://github.com/jfreymuth/pulse)" library

## Help Menu

```
$ go run cmd/audioprism/audioprism.go --help

	┌─┐┬ ┬┌┬┐┬┌─┐┌─┐┬─┐┬┌─┐┌┬┐   ┌─┐┌─┐
	├─┤│ │ ││││ │├─┘├┬┘│└─┐│││───│ ┬│ │
	┴ ┴└─┘─┴┘┴└─┘┴  ┴└─┴└─┘┴ ┴   └─┘└─┘
	Audio Spectrogram Visualization

Available Commands:
  f            with Fyne GUI
  m            with golang.org/x/mobile GUI
  w            with wasm via websockets
```
