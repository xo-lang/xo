package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// htmltokCorpus runs htmltokoracle, its own module (golang.org/x/net is
// std/htmltok's reference, not part of Go's standard library).
func htmltokCorpus() string {
	cmd := exec.Command("go", "run", ".", runtime.Version())
	cmd.Dir = filepath.Join("scripts", "stdcorpus", "htmltokoracle")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		panic("htmltokoracle: " + err.Error())
	}
	return string(out)
}
