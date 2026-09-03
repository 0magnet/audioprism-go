// Package main gen.go
//
// Code generation utilities for command wrappers
package main

import (
	"fmt"
	"github.com/0magnet/audioprism-go/internal/buildinfo"
)

var (
	bv bool
	di bool
)

func init() {
	if fmt.Sprintf("%v", buildinfo.DebugBuildInfo()) != "" {
		RootCmd.Flags().BoolVarP(&di, "info", "d", false, "print runtime/debug.BuildInfo")
	}
	if fmt.Sprintf("%v", buildinfo.DBIVersion()) != "" {
		RootCmd.Flags().BoolVarP(&bv, "bv", "b", false, "print runtime/debug.BuildInfo.Main.Version")
	}
	if buildinfo.DBIVersion() != "" {
		RootCmd.Long += fmt.Sprintf("\n%v", buildinfo.DBIVersion())
	} else {
		RootCmd.Long += fmt.Sprintf("\nversion %v", buildinfo.Version())
	}
	if buildinfo.Go() != "unknown" && buildinfo.Go() != "" {
		RootCmd.Long += "\nbuilt with " + buildinfo.Go()
	}

}
