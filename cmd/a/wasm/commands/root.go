// Package commands cmd/wasm/commands/root.go
package commands

import (
	"bytes"
	_ "embed"
	"encoding/base64"
	"errors"
	"fmt"
	htmpl "html/template"
	"io"
	"io/fs"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/bitfield/script"
	"github.com/gin-gonic/gin"
	"github.com/jfreymuth/pulse"
	"github.com/spf13/cobra"
	"golang.org/x/net/websocket"

	"github.com/0magnet/audioprism-go/pkg/wsaudio"
	"github.com/0magnet/audioprism-go/pkg/wsaudio/wscodec"
	"github.com/0magnet/audioprism-go/pkg/wtaudio"
)

// sampleRate is the rate every audio path in this command produces:
// PulseAudio is asked to record at it and ffmpeg is asked to resample to
// it. It matches sg.SampleRate in the wasm page. The wire format carries
// no rate, so a mismatch shows up as a pitch shift rather than an error,
// which is why /wt-info publishes this number for a human to check.
const sampleRate = 24000

var (
	webPort       int
	webBind       string
	devMode       bool
	tinyGo        bool
	showFPS       bool
	width, height int
	musicDir      string
	shuffle       bool
	enableWT      bool
	wtPort        int
	wtPath        string

	//go:embed wasm_exec.js
	wasmExecJs []byte

	//go:embed bundle.wasm
	wasmBinary []byte

	wasmData               []byte
	wasmPath               string
	htmlPageTemplateData   htmlTemplateData
	tmpl                   *htmpl.Template
	err                    error
	wasmExecLocation       = getGoRoot() + "/lib/wasm/wasm_exec.js"
	tinygowasmExecLocation = strings.TrimSuffix(getGoRoot(), "go") + "tinygo" + "/targets/wasm_exec.js"
	wasmExecScript         string
)

// getGoRoot returns the GOROOT by executing 'go env GOROOT' command
func getGoRoot() string {
	out, err := script.Exec("go env GOROOT").String()
	if err != nil {
		// Fallback to empty string if go command fails
		return ""
	}
	return strings.TrimSpace(out)
}

func init() {
	RootCmd.Flags().IntVarP(&webPort, "port", "p", 8080, "port to serve on")
	RootCmd.Flags().StringVar(&webBind, "bind", "", "address to bind to; empty means every interface (e.g. 127.0.0.1 to serve only locally)")
	RootCmd.Flags().BoolVarP(&showFPS, "fps", "s", false, "show fps in wasm display")
	RootCmd.Flags().StringVarP(&musicDir, "dir", "D", "", "stream audio files from this directory (via ffmpeg) instead of pulseaudio")
	RootCmd.Flags().BoolVarP(&shuffle, "shuffle", "S", false, "shuffle the playlist (only with --dir)")
	RootCmd.Flags().BoolVar(&enableWT, "wt", true,
		"also offer the audio over WebTransport (HTTP/3 over QUIC, UDP) for ?audio=wt; "+
			"the WebSocket is unaffected and stays the default, and a WebTransport that "+
			"fails to start is logged rather than fatal")
	RootCmd.Flags().IntVar(&wtPort, "wt-port", 0,
		"UDP port for WebTransport (0 = the same number as --port; QUIC is UDP so the "+
			"numbers can be shared, and sharing them keeps the browser's origin check happy)")
	RootCmd.Flags().StringVar(&wtPath, "wt-path", "/wt", "WebTransport endpoint path")
	_, err = script.Exec(`bash --help`).Bytes()
	if err == nil {
		_, err = script.Exec(`go help`).Bytes()
		if err == nil {
			RootCmd.Flags().BoolVarP(&devMode, "dev", "d", false, "compile wasm from source")
		}
		_, err1 := script.Exec(`tinygo help`).Bytes()
		if err1 == nil {
			RootCmd.Flags().BoolVarP(&tinyGo, "tinygo", "t", false, "compile wasm from source with tinygo")
		}
		if err == nil || err1 == nil {
			RootCmd.Flags().StringVarP(&wasmPath, "wpath", "w", "cmd/a/wasm/wasm/b.go", "path to wasm source in dev mode")
			RootCmd.Flags().IntVarP(&width, "width", "x", 640, "width of spectrogram display - set on wasm compilation")
			RootCmd.Flags().IntVarP(&height, "height", "y", 480, "height of spectrogram display - set on wasm compilation")
		}
	}

}

type commandTpl struct {
	Tiny     string
	Target   string
	WasmPath string
	Height   string
	Width    string
	LDFlags  string
}

// RootCmd is the root cli command
var RootCmd = &cobra.Command{
	SilenceErrors:         true,
	SilenceUsage:          true,
	DisableSuggestions:    true,
	DisableFlagsInUseLine: true,
	Use:                   "wasm",
	Short:                 "with wasm via websockets",
	Long: `
	┌─┐┬ ┬┌┬┐┬┌─┐┌─┐┬─┐┬┌─┐┌┬┐   ┌─┐┌─┐
	├─┤│ │ ││││ │├─┘├┬┘│└─┐│││───│ ┬│ │
	┴ ┴└─┘─┴┘┴└─┘┴  ┴└─┴└─┘┴ ┴   └─┘└─┘
	` + "Audio Spectrogram Visualization in Webassembly",
	Run: func(_ *cobra.Command, _ []string) {
		if tinyGo {
			devMode = true
			wasmExecScript, err = script.File(tinygowasmExecLocation).String()
			if err != nil {
				log.Fatalf("Could not read tinygo wasm_exec.js file: %s\n", err)
			}
		}
		if devMode && wasmExecScript == "" {
			wasmExecScript, err = script.File(wasmExecLocation).String()
			if err != nil {
				log.Fatalf("Could not read wasm_exec.js file: %s\n", err)
			}
		}
		wg := new(sync.WaitGroup)

		r1 := gin.New()
		r1.Use(gin.Recovery())
		r1.Use(loggingMiddleware())
		r1.GET("/", func(c *gin.Context) {
			c.Writer.Header().Set("Server", "")
			c.Writer.Header().Set("Content-Type", "text/html;charset=utf-8")
			c.Writer.Header().Set("Transfer-Encoding", "chunked")
			c.Writer.WriteHeader(http.StatusOK)
			c.Writer.Flush()
			tmpl, err = htmpl.New("index").Parse(indexHtmpl)
			if err != nil {
				msg := fmt.Sprintf("Error parsing html template indexHtmpl:\n%s\n%v\n", indexHtmpl, err)
				fmt.Println(msg)
				_, err = fmt.Fprintf(c.Writer, `<!DOCTYPE html><html><head><meta charset="utf-8"><title>Error</title></head><body style='background-color: black; color: white;'><div>%s</div></body></html>`, strings.ReplaceAll(msg, "\n", "<br>"))
				if err != nil {
					log.Println(err.Error())
					return
				}
				c.Writer.Flush()
				return
			}

			if devMode {
				execTpl := `bash -c 'GOOS=js GOARCH=wasm {{.Tiny}}go build --ldflags "{{.Height}}{{.Width}}{{.LDFlags}}" {{.Target}}  -o /dev/stdout {{.WasmPath}}'`
				tmpl, err := template.New("exec").Parse(execTpl)
				if err != nil {
					log.Fatalf("Error parsing template: %v", err)
				}
				data := commandTpl{}
				data.WasmPath = wasmPath
				if height != 480 {
					data.Height = " -X='main.height=" + strconv.Itoa(height) + "' "
				}
				if width != 640 {
					data.Width = " -X='main.width=" + strconv.Itoa(width) + "' "
				}
				data.LDFlags = " -s -w "
				if showFPS {
					data.LDFlags += " -X='main.showfps=true' "
				}

				htmlPageTemplateData.WasmExecJs = htmpl.JS(wasmExecScript) //nolint
				if tinyGo {
					data.LDFlags = " -X='main.tinygo=true' "
					if showFPS {
						data.LDFlags += " -X='main.showfps=true' "
					}
					data.Tiny = "tiny"
					data.Target = "-target wasm "
					htmlPageTemplateData.Title = "audioprism-go WASM tinygo dev mode"
				} else {
					htmlPageTemplateData.Title = "audioprism-go WASM dev mode"
				}
				var command bytes.Buffer
				err = tmpl.Execute(&command, data)
				if err != nil {
					log.Fatalf("Error executing template: %v", err)
				}
				fmt.Println(command.String())
				wasmData, err = script.Exec(command.String()).Bytes()

				if err != nil {
					msg := fmt.Sprintf("Could not compile or read wasm file:\n%s\n%v\n", string(wasmData), err)
					fmt.Println(msg)
					_, err = fmt.Fprintf(c.Writer, `<!DOCTYPE html><html><head><meta charset="utf-8"><title>Error</title></head><body style='background-color: black; color: white;'><div>%s</div></body></html>`, strings.ReplaceAll(msg, "\n", "<br>"))
					if err != nil {
						log.Println(err.Error())
						return
					}
					c.Writer.Flush()
					return
				}
				htmlPageTemplateData.WasmBase64 = base64.StdEncoding.EncodeToString(wasmData)
			} else {
				htmlPageTemplateData.Title = "audioprism-go WASM"
				htmlPageTemplateData.WasmExecJs = htmpl.JS(string(wasmExecJs)) //nolint
				htmlPageTemplateData.WasmBase64 = base64.StdEncoding.EncodeToString(wasmBinary)
			}

			tmplData := map[string]interface{}{
				"Page": htmlPageTemplateData,
			}
			var result bytes.Buffer
			err = tmpl.Execute(&result, tmplData)
			if err != nil {
				msg := fmt.Sprintf("Could not execute html template %v\n", err)
				fmt.Println(msg)
				_, err = fmt.Fprintf(c.Writer, `<!DOCTYPE html><html><head><meta charset="utf-8"><title>Error</title></head><body style='background-color: black; color: white;'><div>%s</div></body></html>`, strings.ReplaceAll(msg, "\n", "<br>"))
				if err != nil {
					log.Println(err.Error())
					return
				}
				c.Writer.Flush()
				return
			}
			_, err = c.Writer.Write(result.Bytes())
			if err != nil {
				log.Println(err.Error())
				return
			}
			c.Writer.Flush()
		})

		r1.GET("/ws", func(c *gin.Context) {
			var handler websocket.Handler
			if musicDir != "" {
				handler = websocket.Handler(wsHandlerFiles)
			} else {
				handler = websocket.Handler(wsHandler)
			}
			handler.ServeHTTP(c.Writer, c.Request)
		})

		// The optional WebTransport listener. Everything about it is
		// additive: it fails soft, it is on a different socket (UDP), and
		// no page reaches it without ?audio=wt.
		wt := startWebTransport()
		r1.GET("/wt-info", func(c *gin.Context) {
			if wt == nil {
				// The page reads a 404 here as "this server has no
				// WebTransport" and falls back to the WebSocket, which is
				// exactly right — it is what --wt=false means.
				c.Status(http.StatusNotFound)
				return
			}
			// The hash is not a secret — it is a fingerprint of a public
			// certificate — but the page has to be able to read it from
			// wherever it is served, including another machine, so the
			// URL is built from the page's own Host header.
			c.JSON(http.StatusOK, wt.Info(c.Request.Host))
		})

		wg.Add(1)
		go func() {
			shown := webBind
			if shown == "" {
				shown = "0.0.0.0"
			}
			fmt.Printf("listening on http://%s:%d using gin router\n", shown, webPort)
			err = r1.Run(fmt.Sprintf("%s:%d", webBind, webPort))
			if err != nil {
				log.Println(err.Error())
			}
			wg.Done()
		}()
		wg.Wait()
	},
}

// startPulseCapture opens one PulseAudio record stream and hands every
// chunk of samples to write, returning a function that tears it down.
//
// One PulseAudio client per caller — per WebSocket connection, per
// WebTransport session — keeps the failure of one browser tab from taking
// down the others. write returning an error unwinds the record stream,
// which is how a closed connection stops the capture.
//
// Shared by both transports so there is one capture path. It also no
// longer calls log.Fatal on a PulseAudio that is not there: that used to
// take the whole web server down when a single tab connected, which is a
// large blast radius for "this machine has no sound server".
func startPulseCapture(write func([]float32) error) (func(), error) {
	c, err := pulse.NewClient()
	if err != nil {
		return nil, fmt.Errorf("pulse.NewClient: %w (is PulseAudio/PipeWire running?)", err)
	}
	stream, err := c.NewRecord(pulse.Float32Writer(func(p []float32) (int, error) {
		if len(p) == 0 {
			return 0, nil
		}
		if err := write(p); err != nil {
			return 0, err // closed connection — unwinds the record stream
		}
		return len(p), nil
	}), pulse.RecordSampleRate(sampleRate), pulse.RecordLatency(0.1))
	if err != nil {
		c.Close()
		return nil, fmt.Errorf("NewRecord: %w", err)
	}
	stream.Start()
	return func() {
		stream.Stop()
		c.Close()
	}, nil
}

func wsHandler(ws *websocket.Conn) {
	defer func() {
		if err := ws.Close(); err != nil {
			log.Println("Error closing WebSocket:", err)
		}
	}()

	stop, err := startPulseCapture(func(p []float32) error {
		// Binary frame, raw little-endian float32 — see pkg/wsaudio for
		// why this is no longer base64 in a text frame.
		if err := wscodec.Samples.Send(ws, p); err != nil {
			log.Println("Failed to send message:", err)
			return err
		}
		return nil
	})
	if err != nil {
		log.Println("Audio capture failed:", err)
		return
	}
	defer stop()

	for {
		var msg string
		if err := websocket.Message.Receive(ws, &msg); err != nil {
			log.Println("WebSocket closed:", err)
			break
		}
	}
}

// startFileCapture is startPulseCapture's counterpart for --dir: the
// playlist runs in a goroutine and every chunk goes to write.
func startFileCapture(write func([]float32) error) (func(), error) {
	files, err := listMusicFiles(musicDir)
	if err != nil {
		return nil, fmt.Errorf("scanning %s: %w", musicDir, err)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no audio files found in %s", musicDir)
	}
	closed := make(chan struct{})
	go runPlaylist(files, func(b []byte) error {
		// ffmpeg already wrote the wire layout, and the WebSocket path
		// forwards those bytes untouched. This path decodes them because
		// a Capture deals in samples — one pass over 1200 samples per
		// 50 ms chunk, which is nothing next to the ffmpeg process that
		// produced them.
		return write(wsaudio.BytesToFloat32(b))
	}, closed)
	return func() { close(closed) }, nil
}

// startCapture picks whichever source the flags selected. This is what
// the WebTransport server is handed, so ?audio=wt hears the same thing
// /ws does rather than always falling back to the microphone.
func startCapture(write func([]float32) error) (func(), error) {
	if musicDir != "" {
		return startFileCapture(write)
	}
	return startPulseCapture(write)
}

// startWebTransport brings up the optional QUIC listener, returning nil
// when it is switched off or cannot start.
//
// Failing soft is deliberate: this is an extra transport nothing reaches
// without ?audio=wt, and a machine where UDP cannot be bound must still
// get the WebSocket server that has always worked. The page treats a
// missing /wt-info as "no WebTransport here" and falls back on its own.
func startWebTransport() *wtaudio.Server {
	if !enableWT {
		return nil
	}
	port := wtPort
	if port == 0 {
		port = webPort
	}
	srv, err := wtaudio.New(wtaudio.Config{
		Addr:       fmt.Sprintf("%s:%d", webBind, port),
		Path:       wtPath,
		Capture:    startCapture,
		SampleRate: sampleRate,
	})
	if err != nil {
		log.Printf("WebTransport off: %v (the WebSocket path is unaffected)", err)
		return nil
	}
	go func() {
		if err := srv.ListenAndServe(); err != nil {
			log.Printf("WebTransport listener stopped: %v (the WebSocket path is unaffected)", err)
		}
	}()
	cert := srv.Cert()
	log.Printf("WebTransport on udp/%d%s — open /?audio=wt", port, wtPath)
	log.Printf("  certificate SHA-256 %s (generated this run, valid until %s)",
		cert.Base64(), cert.NotAfter.Format(time.RFC3339))
	log.Printf("  the page reads that from /wt-info; nothing has to be installed in a trust store")
	return srv
}

// wsHandlerFiles streams audio files from musicDir to the websocket using
// ffmpeg to decode/resample anything ffmpeg supports to mono float32 little-
// endian at sg.SampleRate (24kHz). The playlist loops; with shuffle=true it
// is re-shuffled each pass.
func wsHandlerFiles(ws *websocket.Conn) {
	defer func() {
		if err := ws.Close(); err != nil {
			log.Println("Error closing WebSocket:", err)
		}
	}()

	files, err := listMusicFiles(musicDir)
	if err != nil {
		log.Println("Failed to scan music dir:", err)
		return
	}
	if len(files) == 0 {
		log.Println("No audio files found in", musicDir)
		return
	}
	remote := ws.Request().RemoteAddr
	log.Printf("WS open %s: serving %d audio file(s) from %s (shuffle=%v)",
		remote, len(files), musicDir, shuffle)
	defer log.Printf("WS close %s", remote)

	// Detect client disconnect on a separate goroutine so the streaming
	// loop can exit promptly when the browser closes the socket.
	closed := make(chan struct{})
	go func() {
		var msg string
		for {
			if err := websocket.Message.Receive(ws, &msg); err != nil {
				log.Printf("WS %s recv-loop exit: %v", remote, err)
				close(closed)
				return
			}
		}
	}()

	// ffmpeg already wrote the wire layout into the buffer, so this sends
	// the bytes as they are — the base64 step it used to go through was
	// pure overhead on both ends.
	runPlaylist(files, func(b []byte) error { return wscodec.Samples.Send(ws, b) }, closed)
}

// errSinkGone marks a send failure — the client is gone — as opposed to a
// file ffmpeg could not decode. The two want opposite responses: skip a
// bad file and carry on, but stop entirely when there is nobody listening.
// Without the distinction a dead WebTransport session would keep the
// playlist spawning an ffmpeg per track until the session context finally
// closed.
var errSinkGone = errors.New("audio sink is gone")

// runPlaylist loops the playlist, in shuffled order when asked, until
// closed is closed or the sink goes away. Shared by both transports so
// --dir and --shuffle mean the same thing on each.
func runPlaylist(files []string, send func([]byte) error, closed <-chan struct{}) {
	// Shuffling the playback order. Not a security decision, so the cheap
	// generator is the right one.
	rng := rand.New(rand.NewSource(time.Now().UnixNano())) //nolint:gosec
	order := make([]int, len(files))
	for {
		for i := range order {
			order[i] = i
		}
		if shuffle {
			rng.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
		}
		for _, idx := range order {
			select {
			case <-closed:
				return
			default:
			}
			if err := streamFile(send, files[idx], closed); err != nil {
				if errors.Is(err, errSinkGone) {
					return
				}
				// EOF or pipe errors just mean "track ended".
				if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
					continue
				}
				log.Printf("Stream %s: %v", filepath.Base(files[idx]), err)
				select {
				case <-closed:
					return
				default:
				}
			}
		}
	}
}

// audioExtensions are the file extensions we ask ffmpeg to decode. ffmpeg
// itself handles the actual format detection, so this is just to skip
// obvious non-audio files when walking the directory.
var audioExtensions = map[string]bool{
	".mp3": true, ".flac": true, ".ogg": true, ".oga": true, ".opus": true,
	".wav": true, ".m4a": true, ".aac": true, ".wma": true, ".ape": true,
	".mka": true, ".webm": true, ".mp4": true, ".aiff": true, ".aif": true,
}

func listMusicFiles(dir string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if audioExtensions[strings.ToLower(filepath.Ext(path))] {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

// streamFile decodes one file via ffmpeg and pushes mono float32 LE samples
// to send at real-time rate. It shifts the pacing clock back by
// 250ms so the first few chunks fire immediately, prefilling the client's
// jitter buffer; subsequent chunks are paced to wall clock.
//
// send takes the wire bytes rather than samples because that is what
// ffmpeg produces (-f f32le) and what the WebSocket forwards untouched;
// the WebTransport capture is the one caller that decodes them.
func streamFile(send func([]byte) error, path string, closed <-chan struct{}) error {
	log.Println("Now playing:", filepath.Base(path))

	const chunkSamples = 1200 // 50ms
	const chunkBytes = chunkSamples * 4

	// The program is the constant "ffmpeg"; the variable part is the file the
	// user asked to play, passed as an argument rather than through a shell.
	cmd := exec.Command("ffmpeg", //nolint:gosec
		"-hide_banner",
		"-loglevel", "error",
		"-i", path,
		"-ac", "1",
		"-ar", strconv.Itoa(sampleRate),
		"-f", "f32le",
		"-",
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start ffmpeg: %w", err)
	}
	// Watchdog: kill ffmpeg if the client goes away mid-track so we don't
	// leak processes when io.ReadFull is blocked waiting for the pipe.
	done := make(chan struct{})
	go func() {
		select {
		case <-closed:
			_ = cmd.Process.Kill() //nolint:errcheck // shutting the pipeline down; a kill that fails means it already died
		case <-done:
		}
	}()
	defer func() {
		close(done)
		_ = cmd.Process.Kill()    //nolint:errcheck // as above
		_, _ = cmd.Process.Wait() //nolint:errcheck // reaping the child; the exit status is not wanted
	}()

	buf := make([]byte, chunkBytes)
	// Backdate the pacing start so the jitter buffer prefills without delay.
	start := time.Now().Add(-250 * time.Millisecond)
	samplesSent := int64(0)

	for {
		if _, err := io.ReadFull(stdout, buf); err != nil {
			return err
		}
		target := start.Add(time.Duration(samplesSent * int64(time.Second) / int64(sampleRate)))
		if d := time.Until(target); d > 0 {
			select {
			case <-closed:
				return io.EOF
			case <-time.After(d):
			}
		}
		if err := send(buf); err != nil {
			log.Printf("Send failed after %d samples (~%.2fs): %v",
				samplesSent, float64(samplesSent)/float64(sampleRate), err)
			return fmt.Errorf("%w: %w", errSinkGone, err)
		}
		samplesSent += int64(chunkSamples)
	}
}

func loggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		latency := time.Since(start)
		if latency > time.Minute {
			latency = latency.Truncate(time.Second)
		}
		statusCode := c.Writer.Status()
		method := c.Request.Method
		path := c.Request.URL.Path
		statusCodeBackgroundColor := getBackgroundColor(statusCode)
		methodColor := getMethodColor(method)
		fmt.Printf("[AUDIOPRISM-GO] | %s |%s %3d %s| %13v | %15s | %72s |%s %-7s %s %s\n", time.Now().Format("2006/01/02 - 15:04:05"), statusCodeBackgroundColor, statusCode, resetColor(), latency, c.ClientIP(), c.Request.RemoteAddr, methodColor, method, resetColor(), path)
	}
}
func getBackgroundColor(statusCode int) string {
	switch {
	case statusCode >= http.StatusOK && statusCode < http.StatusMultipleChoices:
		return green
	case statusCode >= http.StatusMultipleChoices && statusCode < http.StatusBadRequest:
		return white
	case statusCode >= http.StatusBadRequest && statusCode < http.StatusInternalServerError:
		return yellow
	default:
		return red
	}
}
func getMethodColor(method string) string {
	switch method {
	case http.MethodGet:
		return blue
	case http.MethodPost:
		return cyan
	case http.MethodPut:
		return yellow
	case http.MethodDelete:
		return red
	case http.MethodPatch:
		return green
	case http.MethodHead:
		return magenta
	case http.MethodOptions:
		return white
	default:
		return reset
	}
}

func resetColor() string { return reset }

const (
	green   = "\033[97;42m"
	white   = "\033[90;47m"
	yellow  = "\033[90;43m"
	red     = "\033[97;41m"
	blue    = "\033[97;44m"
	magenta = "\033[97;45m"
	cyan    = "\033[97;46m"
	reset   = "\033[0m"
)

type htmlTemplateData struct {
	Title      string
	WasmExecJs htmpl.JS
	WasmBase64 string
}

const indexHtmpl = `
<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<title>{{.Page.Title}}</title>
<style>
body, html {
margin: 0;
padding: 0;
width: 100%;
height: 100%;
background-color: black;
color: white;
}
#overlay {
padding: 20px;
overflow-y: scroll;
height: 200vh;
position: relative;
z-index: 4;
}
</style>
<script title="wasm_exec.js">
{{.Page.WasmExecJs}}
</script>
<script>
if (!WebAssembly.instantiateStreaming) { // polyfill
  WebAssembly.instantiateStreaming = async (resp, importObject) => {
    const source = await (await resp).arrayBuffer();
    return await WebAssembly.instantiate(source, importObject);
  };
}
const go = new Go();
let mod, inst;

const wasmBase64 = ` + "`{{.Page.WasmBase64}}`;" + `
const wasmBinary = Uint8Array.from(atob(wasmBase64), c => c.charCodeAt(0)).buffer;

WebAssembly.instantiate(wasmBinary, go.importObject).then((result) => {
  mod = result.module;
  inst = result.instance;
  run().then((result) => {
    console.log("Ran WASM: ", result)
  }, (failure) => {
    console.log("Failed to run WASM: ", failure)
  })
});
async function run() {
  await go.run(inst);
  inst = await WebAssembly.instantiate(mod, go.importObject); // reset instance
}
</script>
</head>
<body style="margin: 0; padding: 0; width: 100%; height: 100%; background-color: black; color: white;">
<div id='gocanvas-container' style="position: absolute; width: 100%; height: 100%; pointer-events: none; z-index: 3;">
<canvas id='gocanvas' style="max-width: 100%; max-height: 100%; z-index: 3;"></canvas></div>
</body>
</html>
`

var (
	writePath string
	wasmSrc   string
)

func init() {
	RootCmd.AddCommand(genCmd)
	genCmd.Hidden = true
	genCmd.Flags().StringVarP(&writePath, "path", "p", "cmd/a/wasm/commands/", "path to write files")
	genCmd.Flags().StringVarP(&wasmSrc, "wpath", "w", "cmd/a/wasm/wasm/", "path to wasm source")
	genCmd.Flags().BoolVarP(&tinyGo, "tinygo", "t", false, "use tinygo")
}

var genCmd = &cobra.Command{
	Use:   "gen",
	Short: "update embedded resources",
	Long:  "update wasm_exec.js & recompile wasm binary bundle.wasm",

	Run: func(_ *cobra.Command, _ []string) {
		wasmSourceFiles, err := script.ListFiles(wasmSrc).String()
		if err != nil {
			log.Fatal(err)
		}
		wasmXecPath := wasmExecLocation
		if tinyGo {
			wasmXecPath = tinygowasmExecLocation
		}
		fmt.Println("copying " + wasmXecPath + " to " + writePath + "wasm_exec.js")
		_, err = script.File(wasmXecPath).WriteFile(writePath + "wasm_exec.js")
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("compiling wasm binary")
		if tinyGo {
			fmt.Println(`bash -c "GOOS=js GOARCH=wasm tinygo build --ldflags '-X=main.tinygo=true' -target wasm  -o ` + writePath + `bundle.wasm ` + strings.TrimRight(wasmSourceFiles, "\r\n") + `"`)
			_, err = script.Exec(`bash -c "GOOS=js GOARCH=wasm tinygo build --ldflags '-X=main.tinygo=true' -target wasm  -o ` + writePath + `bundle.wasm ` + strings.TrimRight(wasmSourceFiles, "\r\n") + `"`).Stdout()
			if err != nil {
				log.Fatal(err)
			}
		} else {
			fmt.Println(`bash -c "GOOS=js GOARCH=wasm go build --ldflags '-s -w'  -o ` + writePath + `bundle.wasm ` + strings.TrimRight(wasmSourceFiles, "\r\n") + `"`)
			_, err = script.Exec(`bash -c "GOOS=js GOARCH=wasm go build --ldflags '-s -w'  -o ` + writePath + `bundle.wasm ` + strings.TrimRight(wasmSourceFiles, "\r\n") + `"`).Stdout()
			if err != nil {
				log.Fatal(err)
			}
		}
	},
}
