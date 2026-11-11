package main

import (
	"os"
	"os/exec"
	"path/filepath"
)

// sseCorpus runs sseoracle, its own module (github.com/tmaxmax/go-sse is
// its cross-check, not part of Go's standard library).
func sseCorpus() string {
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = filepath.Join("scripts", "stdcorpus", "sseoracle")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		panic("sseoracle: " + err.Error())
	}
	return string(out)
}
