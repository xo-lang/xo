package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// htmlparseCorpus runs htmlparseoracle, its own module (golang.org/x/net is
// std/htmlparse's reference, not part of Go's standard library).
func htmlparseCorpus() string {
	cmd := exec.Command("go", "run", ".", runtime.Version())
	cmd.Dir = filepath.Join("scripts", "stdcorpus", "htmlparseoracle")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		panic("htmlparseoracle: " + err.Error())
	}
	return string(out)
}
