package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// embedfuzzCorpus runs embedoracle, its own module: seeded random paths,
// directories and glob patterns with Go's io/fs answers over the tree of
// std/evidence/embed/stress/data.
func embedfuzzCorpus() string {
	cmd := exec.Command("go", "run", ".", runtime.Version())
	cmd.Dir = filepath.Join("scripts", "stdcorpus", "embedoracle")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		panic("embedoracle: " + err.Error())
	}
	return string(out)
}
