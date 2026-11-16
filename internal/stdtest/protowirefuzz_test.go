// A fuzz harness for std/protowire against protobuf-go's protowire (decision
// 0616's fuzz kind; task #116). scripts/stdcorpus/protowireoracle, its own
// module, writes seeded random wire messages (every wire type, nested groups,
// edge field numbers, varints of every length), half of them changed (cut
// short, a reserved wire type, field number 0, an overlong varint, a length
// past the end, an unmatched end group, a field number past 2^29 or int32),
// groups nested around protowire's 10000 limit, and varints and tags; walk
// must give ConsumeTag and ConsumeFieldValue's fields or error, and wv and
// wtag AppendVarint and AppendTag's bytes.
//
// std/protowire is internal and its Fld fields are std's own (decision
// 0440), so the cases are an in-module test: seed 0 is the committed
// std/protowire_test.xo, which `xo test --std-module=protowire` runs;
// TestProtowireFuzzCorpusCurrent checks it is what the oracle writes
// (XO_PROTOWIREFUZZ_WRITE=1 rewrites it). TestProtowireFuzz writes a fresh
// seed's file (XO_PROTOWIREFUZZ_SEED, default 2) into a copy of std and runs
// it there on XO_PROTOWIREFUZZ_BACKENDS (default arm64 and llvm), as
// TestHttpHeadFuzz does.
package stdtest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

func pwOracle(t *testing.T, seed int) string {
	t.Helper()
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = filepath.Join(root, "scripts", "stdcorpus", "protowireoracle")
	cmd.Env = append(os.Environ(), "XO_PROTOWIREFUZZ_SEED="+strconv.Itoa(seed))
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("protowireoracle: %v", err)
	}
	return string(out)
}

// TestProtowireFuzzCorpusCurrent: std/protowire_test.xo is what seed 0 writes.
func TestProtowireFuzzCorpusCurrent(t *testing.T) {
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	bxCorpusCurrent(t, filepath.Join(root, "std", "protowire_test.xo"), "XO_PROTOWIREFUZZ_WRITE", pwOracle(t, 0))
}

// TestProtowireFuzz runs a fresh seed's cases in a copy of std.
func TestProtowireFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the protowire fuzz is a heavy test; its seed 0 cases are std/protowire_test.xo")
	}
	text := pwOracle(t, envInt(t, "XO_PROTOWIREFUZZ_SEED", 2))
	probeModule(t, "protowire", "protowire_test.xo", text, envBackends("XO_PROTOWIREFUZZ_BACKENDS", []string{"arm64", "llvm"}), strings.Count(text, "\n  ("))
}
