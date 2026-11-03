package main

import (
	"os"
	"os/exec"
	"path/filepath"
)

// idnaCorpus runs idnaoracle, its own module (golang.org/x/net is std/idna's
// reference, not part of Go's standard library).
func idnaCorpus() string {
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = filepath.Join("scripts", "stdcorpus", "idnaoracle")
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOPROXY=off", "GOSUMDB=off")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		panic("idnaoracle: " + err.Error())
	}
	return string(out)
}
