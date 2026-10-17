package gen

import (
	"os"
	"os/exec"

	"github.com/xo-lang/xo/internal/gotool"
)

// The tests of the Go backend: since decision 0215 they build through the
// xo binary (internal/xotool), whose Go backend is compiler/gen; this
// package has no Go code of its own.

// run runs name with args in dir and returns its combined output; a go
// command runs with goVetEnv on the Xo Go cache.
func run(dir, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if name == "go" {
		cmd.Env = goVetEnv()
		defer gotool.HoldCache()()
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// goVetEnv is the environment of a go command run in a generated tree:
// -trimpath keeps the tree's directory out of the cache key (else every
// go vet compiled rt again, 24 MB per program), and the Xo Go cache is
// capped (gotchas/go-vet-recompiles-rt-per-build-dir.md).
func goVetEnv() []string {
	return append(os.Environ(), "GOWORK=off", "GOFLAGS=-trimpath", gotool.CacheEnv())
}
