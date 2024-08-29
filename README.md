# audioprism-go

**Code is WIP**

Port of [audioprism](https://github.com/vsergeev/audioprism) to golang using the following GUI frameworks:

 * [Fyne](https://github.com/fyne-io/fyne)
* [Go mobile](https://pkg.go.dev/golang.org/x/mobile)
* Web Assembly WASM

Support for pulseaudio

```
$ go run cmd/audioprism/audioprism.go --help

	┌─┐┬ ┬┌┬┐┬┌─┐┌─┐┬─┐┬┌─┐┌┬┐   ┌─┐┌─┐
	├─┤│ │ ││││ │├─┘├┬┘│└─┐│││───│ ┬│ │
	┴ ┴└─┘─┴┘┴└─┘┴  ┴└─┴└─┘┴ ┴   └─┘└─┘
	Audio Spectrogram Visualization

Available Commands:
  f            with Fyne GUI
  m            with golang.org/x/mobile GUI
  mw           with golang.org/x/mobile GUI via websockets
  w            with wasm via websockets
```
