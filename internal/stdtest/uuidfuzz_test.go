// A fuzz harness for std/uuid against github.com/google/uuid v1.6.0, its
// reference (decision 0282), for decision 0616's fuzz kind. The cases come
// from scripts/stdcorpus/uuidoracle's fuzz mode (its own module, so the
// repository's go.mod has no third-party requirement): texts near Parse's
// four forms (32 hex digits, dashed, urn:uuid: and braces) with bytes
// changed, cut, recased or replaced by multi-byte text, so every accepted
// length meets every kind of byte at every position; byte strings of 0 to
// 32 bytes for FromBytes; and pairs of UUIDs sharing a prefix for the
// order. Each is checked against Parse (the UUID, its version and bytes, or
// the error text), FromBytes, display parsing back, and bytes.Compare.
// The fixed seed corpus is testdata/corpus/uuid (scripts/stdcorpus, run by
// TestStdCorpora); TestUuidFuzz runs a fresh seed (XO_UUIDFUZZ_SEED,
// default 2; XO_UUIDFUZZ_N texts, default 2000) on XO_UUIDFUZZ_BACKENDS
// (default arm64 and llvm).
package stdtest

import (
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

func TestUuidFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the uuid fuzz is a heavy test; its fixed corpus runs in TestStdCorpora")
	}
	seed := envInt(t, "XO_UUIDFUZZ_SEED", 2)
	n := envInt(t, "XO_UUIDFUZZ_N", 2000)
	cmd := exec.Command("go", "run", ".", "fuzz", strconv.Itoa(seed), strconv.Itoa(n))
	cmd.Dir = filepath.Join("..", "..", "scripts", "stdcorpus", "uuidoracle")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("uuidoracle: %v", err)
	}
	runFuzzDir(t, "uuidfuzz_test.xo", string(out), envBackends("XO_UUIDFUZZ_BACKENDS", []string{"arm64", "llvm"}), n+n/4+n/4)
}
