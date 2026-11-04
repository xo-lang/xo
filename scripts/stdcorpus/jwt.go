package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// jwtCorpus runs jwtoracle, its own module (golang.org/x/crypto is
// std/jwt's reference, not part of Go's standard library).
func jwtCorpus() string {
	cmd := exec.Command("go", "run", ".", runtime.Version())
	cmd.Dir = filepath.Join("scripts", "stdcorpus", "jwtoracle")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		panic("jwtoracle: " + err.Error())
	}
	return string(out)
}
