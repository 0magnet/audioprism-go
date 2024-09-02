// cmd/audioprism/audioprism.go
//go:generate go run cmd/audioprism/audioprism.go gen -w
package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/bitfield/script"
	cc "github.com/ivanpirog/coloredcobra"
	"github.com/spf13/cobra"

	fyneui "github.com/0magnet/audioprism-go/cmd/fyne/commands"
	gomobileui "github.com/0magnet/audioprism-go/cmd/gomobile/commands"
	wasm "github.com/0magnet/audioprism-go/cmd/wasm/commands"
)

func init() {
	RootCmd.AddCommand(
		fyneui.RootCmd,
		gomobileui.RootCmd,
		wasm.RootCmd,
		genCmd,
	)
	genCmd.Hidden = true
	fyneui.RootCmd.Use = "f"
	fyneui.RootCmd.Long = `
	┌─┐┬ ┬┌┐┌┌─┐
	├┤ └┬┘│││├┤
	└   ┴ ┘└┘└─┘
	` + "Audio Spectrogram Visualization with Fyne GUI"
	gomobileui.RootCmd.Use = "m"
	gomobileui.RootCmd.Long = `
	┌─┐┌─┐┌┬┐┌─┐┌┐ ┬┬  ┌─┐
	│ ┬│ │││││ │├┴┐││  ├┤
	└─┘└─┘┴ ┴└─┘└─┘┴┴─┘└─┘
	` + "Audio Spectrogram Visualization with golang.org/x/mobile GUI"
	wasm.RootCmd.Use = "w"
	wasm.RootCmd.Long = `
	┬ ┬┌─┐┌─┐┌┬┐
	│││├─┤└─┐│││
	└┴┘┴ ┴└─┘┴ ┴
	` + "Audio Spectrogram Visualization in Webassembly"
	var helpflag bool
	RootCmd.SetUsageTemplate(help)
	RootCmd.PersistentFlags().BoolVarP(&helpflag, "help", "h", false, "help for "+RootCmd.Use)
	RootCmd.SetHelpCommand(&cobra.Command{Hidden: true})
	RootCmd.PersistentFlags().MarkHidden("help") //nolint
	RootCmd.CompletionOptions.DisableDefaultCmd = true
	RootCmd.SetUsageTemplate(help)

}

// RootCmd contains literally every 'command' from four repos here
var RootCmd = &cobra.Command{
	SilenceErrors:         true,
	SilenceUsage:          true,
	DisableSuggestions:    true,
	DisableFlagsInUseLine: true,
	Use: func() string {
		return strings.Split(filepath.Base(strings.ReplaceAll(strings.ReplaceAll(fmt.Sprintf("%v", os.Args), "[", ""), "]", "")), " ")[0]
	}(),
	Short: "Audio Spectrogram Visualization",
	Long: strings.Split(filepath.Base(strings.ReplaceAll(strings.ReplaceAll(fmt.Sprintf("%v", os.Args), "[", ""), "]", "")), " ")[0] + `
	┌─┐┬ ┬┌┬┐┬┌─┐┌─┐┬─┐┬┌─┐┌┬┐   ┌─┐┌─┐
	├─┤│ │ ││││ │├─┘├┬┘│└─┐│││───│ ┬│ │
	┴ ┴└─┘─┴┘┴└─┘┴  ┴└─┴└─┘┴ ┴   └─┘└─┘
	` + "Audio Spectrogram Visualization",
}

func main() {
	cc.Init(&cc.Config{
		RootCmd:         RootCmd,
		Headings:        cc.HiBlue + cc.Bold,
		Commands:        cc.HiBlue + cc.Bold,
		CmdShortDescr:   cc.HiBlue,
		Example:         cc.HiBlue + cc.Italic,
		ExecName:        cc.HiBlue + cc.Bold,
		Flags:           cc.HiBlue + cc.Bold,
		FlagsDescr:      cc.HiBlue,
		NoExtraNewlines: true,
		NoBottomNewline: true,
	})
	if err := RootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

const help = `{{if gt (len .Aliases) 0}}{{.NameAndAliases}}{{end}}{{if .HasAvailableSubCommands}}Available Commands:{{range .Commands}}{{if (or .IsAvailableCommand)}}
{{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}
Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}
Global Flags:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}

`

var (
	path     string
	writeOut bool
)

func init() {
	genCmd.Flags().StringVarP(&path, "path", "p", "cmd/", "path to commands")
	genCmd.Flags().BoolVarP(&writeOut, "write", "w", false, "write files ; false for preview")
}

type tplData struct {
	Name string
}

var genCmd = &cobra.Command{
	Use:   "gen",
	Short: "Generate a subcommands from template",
	Run: func(_ *cobra.Command, _ []string) {

		tmpl, err := template.New("main").Parse(command)
		if err != nil {
			log.Fatal("Error parsing template:", err)
		}
		dirs, err := script.ListFiles(path).Replace(path, "").Reject(strings.Split(filepath.Base(strings.ReplaceAll(strings.ReplaceAll(fmt.Sprintf("%v", os.Args), "[", ""), "]", "")), " ")[0]).Slice()
		if err != nil {
			log.Fatal(err)
		}
		for _, dir := range dirs {
			var buf bytes.Buffer
			err = tmpl.Execute(&buf, tplData{
				Name: dir,
			})
			if err != nil {
				log.Fatal(err)
			}
			if writeOut {
				_, err := script.Echo(buf.String() + "\n" + "const help = `" + help + "`").WriteFile(path + dir + "/" + dir + ".go")
				if err != nil {
					log.Fatal(err)
				}
			} else {
				_, err := script.Echo("===>" + path + dir + "/" + dir + ".go<===\n" + buf.String() + "\n" + "const help = `" + help + "`").Stdout()
				if err != nil {
					log.Fatal(err)
				}
			}
		}
	},
}

const command = `
// Package main cmd/{{.Name}}/{{.Name}}.go
//CREATED BY GO GENERATE DO NOT EDIT!
package main

import (
	cc "github.com/ivanpirog/coloredcobra"
	"github.com/spf13/cobra"

	"github.com/0magnet/audioprism-go/cmd/{{.Name}}/commands"
)

func init() {
	var helpflag bool
	commands.RootCmd.SetUsageTemplate(help)
	commands.RootCmd.PersistentFlags().BoolVarP(&helpflag, "help", "h", false, "help menu")
	commands.RootCmd.SetHelpCommand(&cobra.Command{Hidden: true})
	commands.RootCmd.PersistentFlags().MarkHidden("help") //nolint
}

func main() {
	cc.Init(&cc.Config{
		RootCmd:         commands.RootCmd,
		Headings:        cc.HiBlue + cc.Bold,
		Commands:        cc.HiBlue + cc.Bold,
		CmdShortDescr:   cc.HiBlue,
		Example:         cc.HiBlue + cc.Italic,
		ExecName:        cc.HiBlue + cc.Bold,
		Flags:           cc.HiBlue + cc.Bold,
		FlagsDescr:      cc.HiBlue,
		NoExtraNewlines: true,
		NoBottomNewline: true,
	})
	if err := commands.RootCmd.Execute(); err != nil {
		panic(err)
	}
}
`
