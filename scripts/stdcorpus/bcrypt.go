package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// bcryptCorpus runs bcryptoracle, its own module (golang.org/x/crypto is
// std/bcrypt's reference, not part of Go's standard library).
func bcryptCorpus() string {
	cmd := exec.Command("go", "run", ".", runtime.Version())
	cmd.Dir = filepath.Join("scripts", "stdcorpus", "bcryptoracle")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		panic("bcryptoracle: " + err.Error())
	}
	return string(out)
}
