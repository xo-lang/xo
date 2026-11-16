// A fresh seed fuzz run for std/template against Go's text/template (task
// #108, decision 0616's fuzz kind). The cases are the corpus generator's
// (scripts/stdcorpus/template.go: seeded random templates of pipelines,
// builtin functions, if, with, range with break and continue, variables,
// trim markers, define, block, template and bad syntax, over one JSON
// document), made with XO_TEMPLATEFUZZ_SEED added to its seed and
// XO_TEMPLATEFUZZ_N templates (default 2000); seed 0 is the committed
// testdata/corpus/template, which TestStdCorpora runs. Xo must parse and
// render each as Go does: the output, or the error's text. TestTemplateFuzz
// runs XO_TEMPLATEFUZZ_SEED (default 2) on XO_TEMPLATEFUZZ_BACKENDS (default
// arm64 and llvm).
package stdtest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

func TestTemplateFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the template fuzz is a heavy test; its seed 0 corpus runs in TestStdCorpora")
	}
	seed := envInt(t, "XO_TEMPLATEFUZZ_SEED", 2)
	n := envInt(t, "XO_TEMPLATEFUZZ_N", 2000)
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	cmd := exec.Command("go", "run", "./scripts/stdcorpus", "template")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "XO_TEMPLATEFUZZ_SEED="+strconv.Itoa(seed), "XO_TEMPLATEFUZZ_N="+strconv.Itoa(n), "XO_STDCORPUS_OUT="+out)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("scripts/stdcorpus template: %v\n%s", err, b)
	}
	text, err := os.ReadFile(filepath.Join(out, "template_test.xo"))
	if err != nil {
		t.Fatal(err)
	}
	runFuzzDir(t, "template_test.xo", string(text), envBackends("XO_TEMPLATEFUZZ_BACKENDS", []string{"arm64", "llvm"}), n)
}
