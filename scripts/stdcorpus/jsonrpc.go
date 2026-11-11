package main

import (
	"os"
	"os/exec"
	"path/filepath"
)

// jsonrpcCorpus runs jsonrpcoracle, its own module (github.com/modelcontextprotocol/go-sdk is
// its cross-check, not part of Go's standard library).
func jsonrpcCorpus() string {
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = filepath.Join("scripts", "stdcorpus", "jsonrpcoracle")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		panic("jsonrpcoracle: " + err.Error())
	}
	return string(out)
}
