// Package main gen.go
//
// Code generation utilities for command wrappers
package main

import (
	"bytes"
	"log"
	"text/template"

	"github.com/0magnet/calvin"
	"github.com/bitfield/script"
	"github.com/spf13/cobra"
)

//command wrapper generation
//any directories in the specified path will have a .go file of the same name written into them with the rendered wrapper template
//assumes that these dirs contain a 'commands' package / subdir

var (
	path     string
	writeOut bool
)

func init() {
	RootCmd.AddCommand(genCmd)
	genCmd.Hidden = true
	genCmd.Flags().StringVarP(&path, "path", "p", "cmd/a", "path to commands")
	genCmd.Flags().BoolVarP(&writeOut, "write", "w", false, "write files ; false for preview")
}

type tplData struct {
	Name string
	Long string
	Path string
}

var genCmd = &cobra.Command{
	Use:   "gen",
	Short: "generate subcommands from template",
	Long:  "generating subcommands from template should only be attempted when run from source",
	Run: func(_ *cobra.Command, _ []string) {

		cmdTmpl, err := template.New("main").Parse(command)
		if err != nil {
			log.Fatal("Error parsing template:", err)
		}
		dirs, err := script.ListFiles(path).Basename().Slice()
		if err != nil {
			log.Fatal(err)
		}
		for _, dir := range dirs {
			var buf bytes.Buffer
			err = cmdTmpl.Execute(&buf, tplData{
				Name: dir,
				Path: path,
			})
			if err != nil {
				log.Fatal(err)
			}
			if writeOut {
				_, err := script.Echo(buf.String()).WriteFile(path + "/" + dir + "/" + dir + ".go")
				if err != nil {
					log.Fatal(err)
				}
			} else {
				_, err := script.Echo("===>" + path + "/" + dir + "/" + dir + ".go<===\n" + buf.String()).Stdout()
				if err != nil {
					log.Fatal(err)
				}
			}
		}

		cmdsTmpl, err := template.New("niam").Parse(commands)
		if err != nil {
			log.Fatal("Error parsing template:", err)
		}
		// don't overwrite custom implementations
		dirs, err = script.ListFiles(path).Basename().Reject("wasm").Reject("coreweb").Slice()
		if err != nil {
			log.Fatal(err)
		}
		for _, dir := range dirs {
			var buf bytes.Buffer
			err = cmdsTmpl.Execute(&buf, tplData{
				Name: dir,
				Long: calvin.AsciiFont(dir),
			})
			if err != nil {
				log.Fatal(err)
			}
			if writeOut {
				_, err := script.Echo(buf.String()).WriteFile(path + "/" + dir + "/commands/root.go")
				if err != nil {
					log.Fatal(err)
				}
			} else {
				_, err := script.Echo("===>" + path + "/" + dir + "/commands/root.go<===\n" + buf.String() + "`\n").Stdout()
				if err != nil {
					log.Fatal(err)
				}
			}
		}
	},
}

const command = `// Package main {{.Path}}/{{.Name}}/{{.Name}}.go
//CREATED WITH GO GENERATE DO NOT EDIT!
package main

import (
	"github.com/0magnet/audioprism-go/{{.Path}}/{{.Name}}/commands"
	"github.com/0magnet/audioprism-go/internal/flags"
)

func init() {
	flags.InitFlags(commands.RootCmd, true)
}

func main() {
	if err := commands.RootCmd.Execute(); err != nil {
		panic(err)
	}
}
`

const commands = `// Package commands {{.Path}}/{{.Name}}/commands/root.go
//CREATED WITH GO GENERATE DO NOT EDIT!
package commands

import (
	"github.com/spf13/cobra"

	sg "github.com/0magnet/audioprism-go/pkg/spectrogram"
	ui "github.com/0magnet/audioprism-go/pkg/ui/{{.Name}}"
)

var (
	w, h, u, b int
	s          bool
	k          string
	colors     string
	winFunc    string
	magScale   string
	magMin     float64
	magMax     float64
	dftSize    int
	overlap    float64
)

func init() {
	RootCmd.Flags().IntVarP(&w, "width", "x", 640, "initial window width")
	RootCmd.Flags().IntVarP(&h, "height", "y", 480, "initial window height")
	RootCmd.Flags().IntVarP(&u, "up", "u", 60, "fps rate - 0 unlimits")
	RootCmd.Flags().IntVarP(&b, "buf", "b", 32768, "size of audio buffer")
	RootCmd.Flags().BoolVarP(&s, "fps", "s", false, "show fps")
	RootCmd.Flags().StringVarP(&k, "websocket", "k", "", "websocket url (i.e. 'ws://127.0.0.1:8080/ws')")
	RootCmd.Flags().StringVar(&colors, "colors", "heat", "color scheme: heat, blue, grayscale, turbo, viridis, magma")
	RootCmd.Flags().StringVar(&winFunc, "window", "hann", "window function: hann, hamming, bartlett, rectangular")
	RootCmd.Flags().StringVar(&magScale, "magnitude-scale", "log", "magnitude scale: log, linear")
	RootCmd.Flags().Float64Var(&magMin, "magnitude-min", 0.0, "magnitude minimum")
	RootCmd.Flags().Float64Var(&magMax, "magnitude-max", 45.0, "magnitude maximum")
	RootCmd.Flags().IntVar(&dftSize, "dft-size", 1024, "DFT size (power of 2, 64-8192)")
	RootCmd.Flags().Float64Var(&overlap, "overlap", 0.50, "samples overlap percentage (5-95), or a ratio (0.05-0.95)")
}

// RootCmd contains the root command
var RootCmd = &cobra.Command{
	SilenceErrors:         true,
	SilenceUsage:          true,
	DisableSuggestions:    true,
	DisableFlagsInUseLine: true,
	Use:                   "{{.Name}}",
	Short:                 "with {{.Name}}",
	Long: ` + "`" + `{{.Long}}` + "`" + ` + "\nAudio Spectrogram Visualization with {{.Name}}",
	Run: func(_ *cobra.Command, _ []string) {
		sg.S.SetColorByName(colors)
		sg.S.SetWindowByName(winFunc)
		sg.S.SetScaleByName(magScale)
		sg.S.SetMagMin(magMin)
		sg.S.SetMagMax(magMax)
		sg.S.SetDFTSize(dftSize)
		sg.S.SetOverlap(sg.OverlapFromFlag(overlap))
		ui.Run(w, h, u, b, s, k)
	},
}
`
