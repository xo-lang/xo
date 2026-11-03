package main

import (
	"os"
	"os/exec"
	"path/filepath"
)

// normCorpus runs normoracle, its own module (golang.org/x/text is
// std/norm's reference, not part of Go's standard library).
func normCorpus() string {
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = filepath.Join("scripts", "stdcorpus", "normoracle")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		panic("normoracle: " + err.Error())
	}
	return string(out)
}
