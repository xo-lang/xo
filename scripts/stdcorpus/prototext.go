package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// prototextCorpus runs prototextoracle, its own module (google.golang.org/protobuf is
// std/prototext's reference, not part of Go's standard library).
func prototextCorpus() string {
	cmd := exec.Command("go", "run", ".", runtime.Version())
	cmd.Dir = filepath.Join("scripts", "stdcorpus", "prototextoracle")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		panic("prototextoracle: " + err.Error())
	}
	return string(out)
}
