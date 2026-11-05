package main

import (
	"os"
	"os/exec"
	"path/filepath"
)

// amqpCorpus runs amqporacle, its own module (github.com/Azure/go-amqp is std/amqp's reference,
// not part of Go's standard library).
func amqpCorpus() string {
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = filepath.Join("scripts", "stdcorpus", "amqporacle")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		panic("amqporacle: " + err.Error())
	}
	return string(out)
}
