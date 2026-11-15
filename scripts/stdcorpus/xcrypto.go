package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// xcryptoCorpus runs xcryptooracle, its own module (golang.org/x/crypto is
// the reference of std/crypto's BLAKE2b, legacy Keccak and
// XChaCha20-Poly1305, not part of Go's standard library).
func xcryptoCorpus() string {
	cmd := exec.Command("go", "run", ".", runtime.Version())
	cmd.Dir = filepath.Join("scripts", "stdcorpus", "xcryptooracle")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		panic("xcryptooracle: " + err.Error())
	}
	return string(out)
}
