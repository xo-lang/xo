package main

import (
	"os"
	"os/exec"
	"path/filepath"
)

// cronCorpus runs cronoracle, its own module (github.com/robfig/cron/v3 is
// std/cron's reference, not part of Go's standard library).
func cronCorpus() string {
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = filepath.Join("scripts", "stdcorpus", "cronoracle")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		panic("cronoracle: " + err.Error())
	}
	return string(out)
}

// cronfuzzCorpus is cronoracle's fuzz mode (seed 1, 1,500 cases): robfig's
// answer for each seeded random spec, parser and start (TestCronFuzz runs
// later seeds).
func cronfuzzCorpus() string {
	cmd := exec.Command("go", "run", ".", "fuzz", "1", "1500")
	cmd.Dir = filepath.Join("scripts", "stdcorpus", "cronoracle")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		panic("cronoracle fuzz: " + err.Error())
	}
	return string(out)
}
