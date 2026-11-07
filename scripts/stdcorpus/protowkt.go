package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// protowktCorpus runs protowktoracle, its own module (google.golang.org/protobuf is
// std/protowkt's reference, not part of Go's standard library).
func protowktCorpus() string {
	cmd := exec.Command("go", "run", ".", runtime.Version())
	cmd.Dir = filepath.Join("scripts", "stdcorpus", "protowktoracle")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		panic("protowktoracle: " + err.Error())
	}
	return string(out)
}
