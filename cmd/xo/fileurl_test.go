package main

import (
	"path/filepath"
	"runtime"
)

// fileURL is p as a file URL: file:///C:/x for a Windows drive path (Go's
// GOPROXY and xo lsp read it so), file:///x elsewhere. "file://"+p named a
// host C: on Windows (winarm VM, 2026-11-09).
func fileURL(p string) string {
	s := filepath.ToSlash(p)
	if runtime.GOOS == "windows" && len(s) > 0 && s[0] != '/' {
		return "file:///" + s
	}
	return "file://" + s
}
