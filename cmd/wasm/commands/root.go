// Package commands cmd/wasm/commands/root.go
//
//go:generate go run ../../audioprism/audioprism.go w gen -p "" -w ../wasm/
package commands

import (
	"bytes"
	_ "embed"
	"encoding/base64"
	"fmt"
	htmpl "html/template"
	"log"
	"math"
	"net/http"
	"runtime"
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
)

var (
	webPort       int
	devMode       bool
	tinyGo        bool
	width, height int

	//go:embed wasm_exec.js
	wasmExecJs []byte

	//go:embed bundle.wasm
	wasmBinary []byte

	wasmData               []byte
	wasmPath               string
	htmlPageTemplateData   htmlTemplateData
	tmpl                   *htmpl.Template
	err                    error
	wasmExecLocation       = runtime.GOROOT() + "/misc/wasm/wasm_exec.js"
	tinygowasmExecLocation = strings.TrimSuffix(runtime.GOROOT(), "go") + "tinygo" + "/targets/wasm_exec.js"
	wasmExecScript         string
)

func init() {
	RootCmd.Flags().IntVarP(&webPort, "port", "p", 8080, "port to serve on")
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
			RootCmd.Flags().StringVarP(&wasmPath, "wpath", "w", "cmd/wasm/wasm/b.go", "path to wasm source in dev mode")
			RootCmd.Flags().IntVarP(&width, "width", "x", 512, "width of spectrogram display - set on wasm compilation")
			RootCmd.Flags().IntVarP(&height, "height", "y", 512, "height of spectrogram display - set on wasm compilation")
		}
	}

}

type commandTpl struct {
	Tiny     string
	Target   string
	WasmPath string
	Height   string
	Width    string
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
				log.Fatal("Could not read tinygo wasm_exec.js file: %s\n", err)
			}
		}
		if devMode && wasmExecScript == "" {
			wasmExecScript, err = script.File(wasmExecLocation).String()
			if err != nil {
				log.Fatal("Could not read wasm_exec.js file: %s\n", err)
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
				_, err = c.Writer.Write([]byte(fmt.Sprintf(`<!DOCTYPE html><html><head><meta charset="utf-8"><title>Error</title></head><body style='background-color: black; color: white;'><div>%s</div></body></html>`, strings.ReplaceAll(msg, "\n", "<br>"))))
				if err != nil {
					log.Println(err.Error())
					return
				}
				c.Writer.Flush()
				return
			}

			if devMode {
				execTpl := `bash -c 'GOOS=js GOARCH=wasm {{.Tiny}}go build -trimpath --ldflags "{{.Height}}{{.Width}}-s -w" {{.Target}} -o /dev/stdout {{.WasmPath}}'`
				tmpl, err := template.New("exec").Parse(execTpl)
				if err != nil {
					log.Fatalf("Error parsing template: %v", err)
				}
				data := commandTpl{}
				data.WasmPath = wasmPath
				if height != 512 {
					data.Height = " -X='main.height=" + strconv.Itoa(height) + "' "
				}
				if width != 512 {
					data.Width = " -X='main.width=" + strconv.Itoa(width) + "' "
				}

				htmlPageTemplateData.WasmExecJs = htmpl.JS(wasmExecScript) //nolint
				if tinyGo {
					data.Tiny = "tiny"
					data.Target = "-target wasm"
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
					_, err = c.Writer.Write([]byte(fmt.Sprintf(`<!DOCTYPE html><html><head><meta charset="utf-8"><title>Error</title></head><body style='background-color: black; color: white;'><div>%s</div></body></html>`, strings.ReplaceAll(msg, "\n", "<br>"))))
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
				_, err = c.Writer.Write([]byte(fmt.Sprintf(`<!DOCTYPE html><html><head><meta charset="utf-8"><title>Error</title></head><body style='background-color: black; color: white;'><div>%s</div></body></html>`, strings.ReplaceAll(msg, "\n", "<br>"))))
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
			handler := websocket.Handler(wsHandler)
			handler.ServeHTTP(c.Writer, c.Request)
		})

		wg.Add(1)
		go func() {
			fmt.Printf("listening on http://127.0.0.1:%d using gin router\n", webPort)
			err = r1.Run(fmt.Sprintf(":%d", webPort))
			if err != nil {
				log.Println(err.Error())
			}
			wg.Done()
		}()
		wg.Wait()
	},
}

func wsHandler(ws *websocket.Conn) {
	defer func() {
		if err := ws.Close(); err != nil {
			log.Println("Error closing WebSocket:", err)
		}
	}()

	c, err := pulse.NewClient()
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	stream, err := c.NewRecord(pulse.Float32Writer(func(p []float32) (int, error) {
		if len(p) > 0 {
			// err := websocket.Message.Send(ws, float32SliceToByteSlice(p))
			err := websocket.Message.Send(ws, float32SliceToBase64String(p))

			if err != nil {
				log.Println("Failed to send message:", err)
				return 0, err
			}
		}
		return len(p), nil
	}), pulse.RecordLatency(0.1))
	if err != nil {
		log.Fatal(err)
	}

	stream.Start()
	defer stream.Stop()

	for {
		var msg string
		if err := websocket.Message.Receive(ws, &msg); err != nil {
			log.Println("WebSocket closed:", err)
			break
		}
	}
}

func float32SliceToBase64String(floats []float32) string {
	// Convert the float32 slice to a byte slice using base64 encoding
	byteSlice := make([]byte, len(floats)*4)
	for i, f := range floats {
		bits := math.Float32bits(f)
		byteSlice[i*4+0] = byte(bits >> 0)
		byteSlice[i*4+1] = byte(bits >> 8)
		byteSlice[i*4+2] = byte(bits >> 16)
		byteSlice[i*4+3] = byte(bits >> 24)
	}
	return base64.StdEncoding.EncodeToString(byteSlice)
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
	genCmd.Flags().StringVarP(&writePath, "path", "p", "cmd/wasm/commands/", "path to write files")
	genCmd.Flags().StringVarP(&wasmSrc, "wpath", "w", "cmd/wasm/wasm/", "path to wasm source")
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
			fmt.Println(`bash -c "GOOS=js GOARCH=wasm tinygo build -trimpath --ldflags '-s -w' --no-debug -o ` + writePath + `bundle.wasm ` + strings.TrimRight(wasmSourceFiles, "\r\n") + `"`)
			_, err = script.Exec(`bash -c "GOOS=js GOARCH=wasm tinygo build -trimpath --ldflags '-s -w' -o ` + writePath + `bundle.wasm ` + strings.TrimRight(wasmSourceFiles, "\r\n") + `"`).Stdout()
			if err != nil {
				log.Fatal(err)
			}
		} else {
			fmt.Println(`bash -c "GOOS=js GOARCH=wasm go build -trimpath --ldflags '-s -w' -o ` + writePath + `bundle.wasm ` + strings.TrimRight(wasmSourceFiles, "\r\n") + `"`)
			_, err = script.Exec(`bash -c "GOOS=js GOARCH=wasm go build -trimpath --ldflags '-s -w' -o ` + writePath + `bundle.wasm ` + strings.TrimRight(wasmSourceFiles, "\r\n") + `"`).Stdout()
			if err != nil {
				log.Fatal(err)
			}
		}
	},
}
