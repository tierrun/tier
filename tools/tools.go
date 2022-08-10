//go:build tools
// +build tools

// Package tools declares dependencies on tools
package tools

import (
	_ "golang.org/x/tools/cmd/goimports"
	_ "honnef.co/go/tools/cmd/staticcheck"
	_ "tailscale.com/version"
)
