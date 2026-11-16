package main

import (
	"os"
	"os/exec"
	"path/filepath"
)

// tomlCorpus runs tomloracle, its own module (github.com/pelletier/go-toml/v2
// is std/toml's reference, not part of Go's standard library).
func tomlCorpus() string {
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = filepath.Join("scripts", "stdcorpus", "tomloracle")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		panic("tomloracle: " + err.Error())
	}
	return string(out)
}
