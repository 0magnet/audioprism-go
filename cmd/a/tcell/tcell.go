// Package main cmd/a/tcell/tcell.go
// CREATED WITH GO GENERATE DO NOT EDIT!
package main

import (
	"github.com/0magnet/audioprism-go/cmd/a/tcell/commands"
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
