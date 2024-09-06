[![Go Report Card](https://goreportcard.com/badge/github.com/0magnet/audioprism-go)](https://goreportcard.com/report/github.com/0magnet/audioprism-go)

# audioprism-go

**Work In Progress**

Port of [audioprism](https://github.com/vsergeev/audioprism) to golang

## Frontends

 * [Fyne](https://github.com/fyne-io/fyne)
* [Go mobile](https://pkg.go.dev/golang.org/x/mobile)
* [Tcell](github.com/gdamore/tcell)
* Web Assembly WASM

## Audio Source

Support for **pulseaudio** via  "[github.com/jfreymuth/pulse](https://github.com/jfreymuth/pulse)" library

## Help Menus

```
$ go run cmd/fyne/fyne.go --help

	┌─┐┬ ┬┌┬┐┬┌─┐┌─┐┬─┐┬┌─┐┌┬┐   ┌─┐┌─┐
	├─┤│ │ ││││ │├─┘├┬┘│└─┐│││───│ ┬│ │
	┴ ┴└─┘─┴┘┴└─┘┴  ┴└─┴└─┘┴ ┴   └─┘└─┘
	Audio Spectrogram Visualization with Fyne GUI


Flags:
  -b, --buf int            size of audio buffer (default 32768)
  -s, --fps                show fps
  -y, --height int         initial window height (default 512)
  -u, --up int             fps rate - 0 unlimits (default 60)
  -k, --websocket string   websocket url (i.e. 'ws://127.0.0.1:8080/ws')
  -x, --width int          initial window width (default 512)

$ go run cmd/gomobile/gomobile.go --help

	┌─┐┬ ┬┌┬┐┬┌─┐┌─┐┬─┐┬┌─┐┌┬┐   ┌─┐┌─┐
	├─┤│ │ ││││ │├─┘├┬┘│└─┐│││───│ ┬│ │
	┴ ┴└─┘─┴┘┴└─┘┴  ┴└─┴└─┘┴ ┴   └─┘└─┘
	Audio Spectrogram Visualization with golang.org/x/mobile GUI


Flags:
  -b, --buf int            size of audio buffer (default 32768)
  -s, --fps                show fps
  -y, --height int         initial window height (default 512)
  -u, --up int             fps rate - 0 unlimits (default 60)
  -k, --websocket string   websocket url (i.e. 'ws://127.0.0.1:8080/ws')
  -x, --width int          initial window width (default 512)

$ go run cmd/tcell/tcell.go --help

	┌─┐┬ ┬┌┬┐┬┌─┐┌─┐┬─┐┬┌─┐┌┬┐   ┌─┐┌─┐
	├─┤│ │ ││││ │├─┘├┬┘│└─┐│││───│ ┬│ │
	┴ ┴└─┘─┴┘┴└─┘┴  ┴└─┴└─┘┴ ┴   └─┘└─┘
	Audio Spectrogram Visualization with github.com/gdamore/tcell Tcell TUI


Flags:
  -b, --buf int            size of audio buffer (default 32768)
  -s, --fps                show fps
  -y, --height int         initial window height (default 512)
  -u, --up int             fps rate - 0 unlimits (default 60)
  -k, --websocket string   websocket url (i.e. 'ws://127.0.0.1:8080/ws')
  -x, --width int          initial window width (default 512)

$ go run cmd/wasm/wasm.go --help

	┌─┐┬ ┬┌┬┐┬┌─┐┌─┐┬─┐┬┌─┐┌┬┐   ┌─┐┌─┐
	├─┤│ │ ││││ │├─┘├┬┘│└─┐│││───│ ┬│ │
	┴ ┴└─┘─┴┘┴└─┘┴  ┴└─┴└─┘┴ ┴   └─┘└─┘
	Audio Spectrogram Visualization in Webassembly


Flags:
  -d, --dev            compile wasm from source
  -y, --height int     height of spectrogram display - set on wasm compilation (default 512)
  -p, --port int       port to serve on (default 8080)
  -t, --tinygo         compile wasm from source with tinygo
  -x, --width int      width of spectrogram display - set on wasm compilation (default 512)
  -w, --wpath string   path to wasm source in dev mode (default "cmd/wasm/wasm/b.go")

$ go run cmd/audioprism/audioprism.go --help
audioprism
	┌─┐┬ ┬┌┬┐┬┌─┐┌─┐┬─┐┬┌─┐┌┬┐   ┌─┐┌─┐
	├─┤│ │ ││││ │├─┘├┬┘│└─┐│││───│ ┬│ │
	┴ ┴└─┘─┴┘┴└─┘┴  ┴└─┴└─┘┴ ┴   └─┘└─┘
	Audio Spectrogram Visualization

Available Commands:
f            with Fyne GUI
m            with golang.org/x/mobile GUI
t            with tcell TUI
w            with wasm via websockets

$ go run cmd/audioprism/audioprism.go f --help

	┌─┐┬ ┬┌┐┌┌─┐
	├┤ └┬┘│││├┤
	└   ┴ ┘└┘└─┘
	Audio Spectrogram Visualization with Fyne GUI


Flags:
  -b, --buf int            size of audio buffer (default 32768)
  -s, --fps                show fps
  -y, --height int         initial window height (default 512)
  -u, --up int             fps rate - 0 unlimits (default 60)
  -k, --websocket string   websocket url (i.e. 'ws://127.0.0.1:8080/ws')
  -x, --width int          initial window width (default 512)

$ go run cmd/audioprism/audioprism.go m --help

	┌─┐┌─┐┌┬┐┌─┐┌┐ ┬┬  ┌─┐
	│ ┬│ │││││ │├┴┐││  ├┤
	└─┘└─┘┴ ┴└─┘└─┘┴┴─┘└─┘
	Audio Spectrogram Visualization with golang.org/x/mobile GUI


Flags:
  -b, --buf int            size of audio buffer (default 32768)
  -s, --fps                show fps
  -y, --height int         initial window height (default 512)
  -u, --up int             fps rate - 0 unlimits (default 60)
  -k, --websocket string   websocket url (i.e. 'ws://127.0.0.1:8080/ws')
  -x, --width int          initial window width (default 512)

$ go run cmd/audioprism/audioprism.go t --help

	┌┬┐┌─┐┌─┐┬  ┬
	 │ │  ├┤ │  │
	 ┴ └─┘└─┘┴─┘┴─┘
	Audio Spectrogram Visualization with Tcell TUI


Flags:
  -b, --buf int            size of audio buffer (default 32768)
  -s, --fps                show fps
  -y, --height int         initial window height (default 512)
  -u, --up int             fps rate - 0 unlimits (default 60)
  -k, --websocket string   websocket url (i.e. 'ws://127.0.0.1:8080/ws')
  -x, --width int          initial window width (default 512)

$ go run cmd/audioprism/audioprism.go w --help

	┬ ┬┌─┐┌─┐┌┬┐
	│││├─┤└─┐│││
	└┴┘┴ ┴└─┘┴ ┴
	Audio Spectrogram Visualization in Webassembly


Flags:
  -d, --dev            compile wasm from source
  -y, --height int     height of spectrogram display - set on wasm compilation (default 512)
  -p, --port int       port to serve on (default 8080)
  -t, --tinygo         compile wasm from source with tinygo
  -x, --width int      width of spectrogram display - set on wasm compilation (default 512)
  -w, --wpath string   path to wasm source in dev mode (default "cmd/wasm/wasm/b.go")

```
