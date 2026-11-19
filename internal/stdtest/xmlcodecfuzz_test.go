// A fresh seed fuzz run for std/xmlcodec, the helpers `derive Xml`
// generates calls, against Go's encoding/xml (task #108, decision 0616's
// fuzz kind). The cases are two corpus generators':
// scripts/stdcorpus/xmlcodec.go's seeded random documents (parse_root,
// attr_of, direct_text and the number and bool readers) and pieces (the
// encoder's open_path, leaf, chardata and comment), and
// scripts/stdcorpus/xmlderive.go's seeded random documents decoded into a
// struct that derives Xml as Go's Unmarshal, then encoded as Marshal and
// MarshalIndent; a fresh seed adds the hostile dictionary's numbers and
// deep nesting to both (task #352). XO_XMLCODECFUZZ_SEED is added to both
// generators' seeds and XO_XMLCODECFUZZ_N cases (default 2000) are split
// over them; seed 0 is the committed testdata/corpus/xmlcodec and xml,
// which TestStdCorpora runs. TestXmlcodecFuzz runs XO_XMLCODECFUZZ_SEED
// (default 2) on XO_XMLCODECFUZZ_BACKENDS (default arm64 and llvm).
package stdtest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

func TestXmlcodecFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the xmlcodec fuzz is a heavy test; its seed 0 corpora run in TestStdCorpora")
	}
	seed := envInt(t, "XO_XMLCODECFUZZ_SEED", 2)
	n := envInt(t, "XO_XMLCODECFUZZ_N", 2000)
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	cmd := exec.Command("go", "run", "./scripts/stdcorpus", "xmlcodec", "xml")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "XO_XMLCODECFUZZ_SEED="+strconv.Itoa(seed), "XO_XMLCODECFUZZ_N="+strconv.Itoa(n), "XO_STDCORPUS_OUT="+out)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("scripts/stdcorpus xmlcodec xml: %v\n%s", err, b)
	}
	backends := envBackends("XO_XMLCODECFUZZ_BACKENDS", []string{"arm64", "llvm"})
	for _, f := range []string{"xmlcodec_test.xo", "xml_test.xo"} {
		text, err := os.ReadFile(filepath.Join(out, f))
		if err != nil {
			t.Fatal(err)
		}
		t.Run(f, func(t *testing.T) { runFuzzDir(t, f, string(text), backends, n/2) })
	}
}
