// Package main is the audioprism command: every tool in this repository is a
// subcommand of it.
//
//go:generate go run . gen -w -p cmd/a
package main

import "github.com/0magnet/audioprism-go/commands"

func main() { commands.Execute() }
