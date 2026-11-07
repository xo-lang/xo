package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// protojsonCorpus runs protojsonoracle, its own module (google.golang.org/protobuf is
// std/protojson's reference, not part of Go's standard library).
func protojsonCorpus() string {
	cmd := exec.Command("go", "run", ".", runtime.Version())
	cmd.Dir = filepath.Join("scripts", "stdcorpus", "protojsonoracle")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		panic("protojsonoracle: " + err.Error())
	}
	return string(out)
}
