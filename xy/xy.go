// Package main xy/xy.go
package main

import (
	"github.com/0magnet/audioprism-go/internal/flags"
	"github.com/0magnet/audioprism-go/xy/commands"
)

func init() {
	flags.InitFlags(commands.RootCmd, true)
}

func main() {
	if err := commands.RootCmd.Execute(); err != nil {
		panic(err)
	}
}
