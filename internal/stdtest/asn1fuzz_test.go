// A fresh seed fuzz run for std/asn1 against Go's encoding/asn1 (task
// #99, decision 0616's fuzz kind). The cases are the corpus generator's
// (scripts/stdcorpus/asn1.go: seeded random DER trees of every type std/asn1
// reads, two in five changed: a bit flipped, a byte replaced, or cut),
// made with XO_ASN1FUZZ_SEED added to its seed and XO_ASN1FUZZ_N trees
// (default 2000); seed 0 is the committed testdata/corpus/asn1, which
// TestStdCorpora runs. Xo must read each as Go's Unmarshal (the tree or the
// error) and write it back as Marshal. TestAsn1Fuzz runs XO_ASN1FUZZ_SEED
// (default 2) on XO_ASN1FUZZ_BACKENDS (default arm64 and llvm).
package stdtest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

func TestAsn1Fuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the asn1 fuzz is a heavy test; its seed 0 corpus runs in TestStdCorpora")
	}
	seed := envInt(t, "XO_ASN1FUZZ_SEED", 2)
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	cmd := exec.Command("go", "run", "./scripts/stdcorpus", "asn1")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "XO_ASN1FUZZ_SEED="+strconv.Itoa(seed), "XO_ASN1FUZZ_N="+strconv.Itoa(envInt(t, "XO_ASN1FUZZ_N", 2000)), "XO_STDCORPUS_OUT="+out)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("scripts/stdcorpus asn1: %v\n%s", err, b)
	}
	text, err := os.ReadFile(filepath.Join(out, "asn1_test.xo"))
	if err != nil {
		t.Fatal(err)
	}
	runFuzzDir(t, "asn1_test.xo", string(text), envBackends("XO_ASN1FUZZ_BACKENDS", []string{"arm64", "llvm"}), envInt(t, "XO_ASN1FUZZ_N", 2000))
}
