// A fresh seed fuzz run for std/protobuf against google.golang.org/protobuf (decision 0461: a parser of
// untrusted input is Early on testing until it is fuzzed). The cases are the corpus generator's
// (scripts/stdcorpus/protobuf.go: random wire fields with wide varints, truncations and changed copies for
// the wire layer, and typed messages checked against dynamicpb, including unknown field preservation), made
// with XO_PROTOFUZZ_SEED added to its seeds; seed 0 is the committed testdata/corpus/protobuf, which
// TestStdCorpora runs on every corpus backend. TestProtobufFuzz runs XO_PROTOFUZZ_SEED (default 2) on
// XO_PROTOFUZZ_BACKENDS (default arm64 and llvm); it skips when the oracle cannot run.
package stdtest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

func TestProtobufFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the protobuf fuzz is a heavy test; its seed 0 corpus runs in TestStdCorpora")
	}
	seed := envInt(t, "XO_PROTOFUZZ_SEED", 2)
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	cmd := exec.Command("go", "run", "./scripts/stdcorpus", "protobuf")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "XO_PROTOFUZZ_SEED="+strconv.Itoa(seed), "XO_STDCORPUS_OUT="+out, "GOFLAGS=-mod=mod", "GOPROXY=off")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("the oracle (internal/stdtest/protoref) cannot run here: %v\n%s", err, b)
	}
	text, err := os.ReadFile(filepath.Join(out, "protobuf_test.xo"))
	if err != nil {
		t.Fatal(err)
	}
	runFuzzDir(t, "protobuf_test.xo", string(text), envBackends("XO_PROTOFUZZ_BACKENDS", []string{"arm64", "llvm"}), 8942)
}
