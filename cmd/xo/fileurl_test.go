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

// hostExe is name as xo writes a host binary given -o name: with .exe on
// Windows (compiler/xo cache.xo), as is elsewhere.
func hostExe(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}
