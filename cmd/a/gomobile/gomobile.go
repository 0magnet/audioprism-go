//go:build cgo

// Package main cmd/a/gomobile/gomobile.go
// CREATED WITH GO GENERATE DO NOT EDIT!
package main

import (
	"github.com/0magnet/audioprism-go/cmd/a/gomobile/commands"
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
