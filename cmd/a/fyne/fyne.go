// Package main cmd/a/fyne/fyne.go
// CREATED WITH GO GENERATE DO NOT EDIT!
package main

import (
	"github.com/0magnet/audioprism-go/cmd/a/fyne/commands"
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
