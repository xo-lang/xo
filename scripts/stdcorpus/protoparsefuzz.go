package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// protoparsefuzzCorpus runs protoparseoracle, its own module
// (bufbuild/protocompile is std/protoparse's reference, not part of Go's
// standard library): seeded random .proto files and their outlines, or
// protocompile's rejection.
func protoparsefuzzCorpus() string {
	cmd := exec.Command("go", "run", ".", runtime.Version())
	cmd.Dir = filepath.Join("scripts", "stdcorpus", "protoparseoracle")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		panic("protoparseoracle: " + err.Error())
	}
	return string(out)
}

// registered here, as its name is longer than the generators table's
// column
func init() { generators["protoparsefuzz"] = protoparsefuzzCorpus }
