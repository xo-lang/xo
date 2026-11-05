package main

import (
	"os"
	"os/exec"
	"path/filepath"
)

// yamlCorpus runs yamloracle, its own module (gopkg.in/yaml.v3 is std/yaml's
// reference, not part of Go's standard library).
func yamlCorpus() string {
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = filepath.Join("scripts", "stdcorpus", "yamloracle")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		panic("yamloracle: " + err.Error())
	}
	return string(out)
}

// yamlfuzzCorpus is yamloracle's fuzz mode (seed 1, 2,000 documents): yaml.v3's
// answer for each seeded random document.
func yamlfuzzCorpus() string {
	cmd := exec.Command("go", "run", ".", "fuzz", "1", "2000")
	cmd.Dir = filepath.Join("scripts", "stdcorpus", "yamloracle")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		panic("yamloracle fuzz: " + err.Error())
	}
	return string(out)
}
