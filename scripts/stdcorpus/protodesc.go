package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// protodescCorpus runs protodescoracle, its own module (google.golang.org/protobuf is
// std/protodesc's reference, not part of Go's standard library).
func protodescCorpus() string {
	cmd := exec.Command("go", "run", ".", runtime.Version())
	cmd.Dir = filepath.Join("scripts", "stdcorpus", "protodescoracle")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		panic("protodescoracle: " + err.Error())
	}
	return string(out)
}
