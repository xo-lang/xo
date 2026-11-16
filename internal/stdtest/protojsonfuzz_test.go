// A fresh seed fuzz run for std/protojson against protobuf-go's protojson
// (#109, decision 0616's fuzz kind). The cases are the corpus generator's
// (scripts/stdcorpus/protojsonoracle: seeded random dynamicpb messages of
// every field kind and the well known types, marshaled with random options
// and read back, and two in five unmarshal inputs changed: a bit flipped, a
// byte replaced, or cut), made with XO_PROTOJSONFUZZ_SEED added to its seed
// and XO_PROTOJSONFUZZ_N messages (default 400); seed 0 is the committed
// testdata/corpus/protojson, which TestStdCorpora runs. Xo must write each
// message as Go does and read each input as Go's Unmarshal (the message or
// an error). TestProtojsonFuzz runs XO_PROTOJSONFUZZ_SEED (default 2) on
// XO_PROTOJSONFUZZ_BACKENDS (default arm64 and llvm).
package stdtest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

func TestProtojsonFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the protojson fuzz is a heavy test; its seed 0 corpus runs in TestStdCorpora")
	}
	seed := envInt(t, "XO_PROTOJSONFUZZ_SEED", 2)
	if seed == 0 {
		t.Fatal("XO_PROTOJSONFUZZ_SEED=0 is the committed corpus (TestStdCorpora); give a fresh seed")
	}
	n := envInt(t, "XO_PROTOJSONFUZZ_N", 400)
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	cmd := exec.Command("go", "run", "./scripts/stdcorpus", "protojson")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "XO_PROTOJSONFUZZ_SEED="+strconv.Itoa(seed), "XO_PROTOJSONFUZZ_N="+strconv.Itoa(n), "XO_STDCORPUS_OUT="+out)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("scripts/stdcorpus protojson: %v\n%s", err, b)
	}
	text, err := os.ReadFile(filepath.Join(out, "protojson_test.xo"))
	if err != nil {
		t.Fatal(err)
	}
	runFuzzDir(t, "protojson_test.xo", string(text), envBackends("XO_PROTOJSONFUZZ_BACKENDS", []string{"arm64", "llvm"}), n)
}
