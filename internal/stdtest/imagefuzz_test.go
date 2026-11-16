// A fresh seed fuzz run for std/image against Go's image and image/color
// (task #116, decision 0616's fuzz kind). The cases are the corpus
// generator's (scripts/stdcorpus/image.go: seeded random colors through
// every color model, YCbCr and CMYK conversions, palettes, and images
// drawn, read and converted), made with XO_IMAGEFUZZ_SEED added to its
// seeds; seed 0 is the committed testdata/corpus/image, which TestStdCorpora
// runs. TestImageFuzz runs XO_IMAGEFUZZ_SEED (default 2) on
// XO_IMAGEFUZZ_BACKENDS (default arm64 and llvm); its case count is the
// generated table rows (the generator's counts are fixed).
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

func TestImageFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the image fuzz is a heavy test; its seed 0 corpus runs in TestStdCorpora")
	}
	seed := envInt(t, "XO_IMAGEFUZZ_SEED", 2)
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	cmd := exec.Command("go", "run", "./scripts/stdcorpus", "image")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "XO_IMAGEFUZZ_SEED="+strconv.Itoa(seed), "XO_STDCORPUS_OUT="+out)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("scripts/stdcorpus image: %v\n%s", err, b)
	}
	text, err := os.ReadFile(filepath.Join(out, "image_test.xo"))
	if err != nil {
		t.Fatal(err)
	}
	runFuzzDir(t, "image_test.xo", string(text), envBackends("XO_IMAGEFUZZ_BACKENDS", []string{"arm64", "llvm"}), strings.Count(string(text), "\n  ("))
}
