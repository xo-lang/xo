package main

import (
	"os"
	"os/exec"
	"path/filepath"
)

// brotliCorpus runs brotlioracle, its own module (github.com/andybalholm/brotli is
// std/brotli's reference, not part of Go's standard library).
func brotliCorpus() string {
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = filepath.Join("scripts", "stdcorpus", "brotlioracle")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		panic("brotlioracle: " + err.Error())
	}
	return string(out)
}
