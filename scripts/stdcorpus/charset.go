package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// charsetCorpus runs charsetoracle, its own module (golang.org/x/text is
// std/charset's reference, not part of Go's standard library).
func charsetCorpus() string {
	cmd := exec.Command("go", "run", ".", runtime.Version())
	cmd.Dir = filepath.Join("scripts", "stdcorpus", "charsetoracle")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		panic("charsetoracle: " + err.Error())
	}
	return string(out)
}
