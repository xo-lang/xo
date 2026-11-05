// A fuzz harness for std/yaml's typed decode (2026-11-05, decision 0512): the
// documents are seeded random ones built from pieces (every scalar form of each
// field kind, block and flow collections, anchors, aliases, merge keys, tags,
// comments, repeated keys, several documents), each cut short or with a byte
// changed; the oracle is gopkg.in/yaml.v3 v3.0.1, run by
// scripts/stdcorpus/yamloracle (its own module, since go.mod stays without
// dependencies). Every document must decode to the value or fail with the
// error text yaml.v3 gives. A document yaml.v3 itself panics on is left out.
//
// The committed seed 1 corpus is testdata/corpus/yamlfuzz (scripts/stdcorpus
// yamlfuzz writes it; TestStdCorpora runs it on every backend). TestYamlFuzz
// runs a fresh seed (XO_YAMLFUZZ_SEED, default 2; XO_YAMLFUZZ_N, default 2,000)
// on XO_YAMLFUZZ_BACKENDS (default arm64 and llvm); it needs the Go module
// cache to hold yaml.v3 (or network access) and is skipped without it.
package stdtest

import (
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestYamlFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the yaml fuzz is a heavy test; its seed 1 corpus runs in TestStdCorpora")
	}
	seed := envInt(t, "XO_YAMLFUZZ_SEED", 2)
	n := envInt(t, "XO_YAMLFUZZ_N", 2000)
	cmd := exec.Command("go", "run", ".", "fuzz", strconv.Itoa(seed), strconv.Itoa(n))
	cmd.Dir = filepath.Join("..", "..", "scripts", "stdcorpus", "yamloracle")
	out, err := cmd.Output()
	if err != nil {
		t.Skipf("yamloracle cannot run (yaml.v3 not in the module cache?): %v", err)
	}
	text := string(out)
	docs := strings.Count(text, "\n  (\"")
	runFuzzDir(t, "yamlfuzz_test.xo", text, envBackends("XO_YAMLFUZZ_BACKENDS", []string{"arm64", "llvm"}), docs)
}
