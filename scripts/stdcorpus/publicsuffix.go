package main

import (
	"os"
	"os/exec"
	"path/filepath"
)

// publicsuffixCorpus runs psloracle, its own module (golang.org/x/net's
// publicsuffix is std/publicsuffix's reference, not part of Go's standard
// library).
func publicsuffixCorpus() string {
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = filepath.Join("scripts", "stdcorpus", "psloracle")
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOPROXY=off", "GOSUMDB=off")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		panic("psloracle: " + err.Error())
	}
	return string(out)
}
