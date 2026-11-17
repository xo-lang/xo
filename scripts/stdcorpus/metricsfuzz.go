package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// metricsfuzzCorpus runs metricsoracle, its own module: seeded random
// registries and operations with prometheus/client_golang's text
// exposition of the same registry.
func metricsfuzzCorpus() string {
	cmd := exec.Command("go", "run", ".", runtime.Version())
	cmd.Dir = filepath.Join("scripts", "stdcorpus", "metricsoracle")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		panic("metricsoracle: " + err.Error())
	}
	return string(out)
}
