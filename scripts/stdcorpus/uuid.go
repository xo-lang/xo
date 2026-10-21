package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// uuidCorpus runs uuidoracle, its own module (github.com/google/uuid is
// std/uuid's reference, not part of Go's standard library).
func uuidCorpus() string {
	cmd := exec.Command("go", "run", ".", runtime.Version())
	cmd.Dir = filepath.Join("scripts", "stdcorpus", "uuidoracle")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		panic("uuidoracle: " + err.Error())
	}
	return string(out)
}
