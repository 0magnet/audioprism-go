//go:build cgo

// Package main gomobile_cmd.go
package main

import (
	"github.com/0magnet/calvin"

	gomobileui "github.com/0magnet/audioprism-go/cmd/a/gomobile/commands"
)

// The gomobile UI is golang.org/x/mobile, whose GL bindings and event loop are
// C — it does not build without cgo, and so does not build for wasm at all.
// Registering it from a separate file is what lets the same binary still build
// in those configurations, without the one UI that cannot work there.
func init() {
	RootCmd.AddCommand(gomobileui.RootCmd)
	gomobileui.RootCmd.Use = "m"
	gomobileui.RootCmd.Long = calvin.AsciiFont("gomobile") + "\n" + asv + " with golang.org/x/mobile GUI"
}
