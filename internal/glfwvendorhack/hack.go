//go:build required

// Package glfwvendorhack forces `go mod vendor` to retain
// github.com/go-gl/glfw/v3.4/glfw/glfw/include, which contains the Wayland
// protocol headers (xdg-shell-client-protocol.h, etc.) referenced by the
// vendored GLFW C sources. Upstream's own build_cgo_hack.go (in v0.1.0-pre.1)
// only references the GLFW/ subdirectory and forgets the parent include/.
package glfwvendorhack

import (
	_ "github.com/go-gl/glfw/v3.4/glfw/glfw/include"
)
