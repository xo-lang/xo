package main

import (
	"os"
	"os/exec"
	"path/filepath"
)

// sqliteCorpus runs sqliteoracle, its own module (github.com/mattn/go-sqlite3 is std/sqlite's reference,
// not part of Go's standard library).
func sqliteCorpus() string {
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = filepath.Join("scripts", "stdcorpus", "sqliteoracle")
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOPROXY=off", "GOSUMDB=off", "CGO_ENABLED=1")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		panic("sqliteoracle: " + err.Error())
	}
	return string(out)
}
