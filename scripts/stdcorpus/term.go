package main

import (
	"os"
	"os/exec"
	"path/filepath"
)

// termCorpus runs termoracle, its own module (golang.org/x/term is
// std/term's line editor reference, not part of Go's standard library).
func termCorpus() string {
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = filepath.Join("scripts", "stdcorpus", "termoracle")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		panic("termoracle: " + err.Error())
	}
	return string(out)
}
