package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// migratefuzzCorpus runs migrateoracle, its own module (pressly/goose's
// parser is std/migrate's reference, not part of Go's standard library):
// seeded random goose migration files with goose's Up and Down statements
// and transaction mode, or its refusal, and file names with their versions.
func migratefuzzCorpus() string {
	cmd := exec.Command("go", "run", ".", runtime.Version())
	cmd.Dir = filepath.Join("scripts", "stdcorpus", "migrateoracle")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		panic("migrateoracle: " + err.Error())
	}
	return string(out)
}
