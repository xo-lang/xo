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

// jweCorpus is std/jwt's JWE corpus, printed by jweoracle (go-jose v4).
func jweCorpus() string {
	cmd := exec.Command("go", "run", ".", runtime.Version())
	cmd.Dir = filepath.Join("scripts", "stdcorpus", "jweoracle")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		panic("jweoracle: " + err.Error())
	}
	return string(out)
}
