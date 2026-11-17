package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// fakenetfuzzCorpus runs fakenetoracle, its own module: seeded random sets
// of FakeNet patterns and requests with the pattern net/http's ServeMux
// picks for each.
func fakenetfuzzCorpus() string {
	cmd := exec.Command("go", "run", ".", runtime.Version())
	cmd.Dir = filepath.Join("scripts", "stdcorpus", "fakenetoracle")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		panic("fakenetoracle: " + err.Error())
	}
	return string(out)
}
