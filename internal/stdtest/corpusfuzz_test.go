// Fresh seed fuzz runs of std modules whose corpus generators draw seeded
// random cases from a Go reference (task #116, decision 0616's fuzz kind):
// the generator runs with XO_<MOD>FUZZ_SEED added to each of its seeds
// (0, the default there, is the committed corpus TestStdCorpora runs), and
// Xo must give the reference's answers on the new cases. Each Test<Mod>Fuzz
// runs XO_<MOD>FUZZ_SEED (default 2) on XO_<MOD>FUZZ_BACKENDS (default arm64
// and llvm); its case count is the generated table rows.
//
//   - brotli: scripts/stdcorpus/brotlioracle (github.com/andybalholm/brotli),
//     random inputs at every quality and window, and changed streams;
//   - charset: charsetoracle (golang.org/x/text/encoding), random text and
//     bytes through every encoding both ways;
//   - htmlparse: htmlparseoracle (golang.org/x/net/html Parse and
//     ParseFragment), random documents from markup pieces;
//   - htmltok: htmltokoracle (x/net/html's Tokenizer) in each mode;
//   - publicsuffix: psloracle (golang.org/x/net/publicsuffix), random
//     domains.
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

// corpusFuzz runs scripts/stdcorpus GEN with XO_ENV_SEED and runs the
// GEN_test.xo it writes.
func corpusFuzz(t *testing.T, gen, env string) {
	if testing.Short() {
		t.Skip("a corpus fuzz is a heavy test; its seed 0 corpus runs in TestStdCorpora")
	}
	seed := envInt(t, "XO_"+env+"_SEED", 2)
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	cmd := exec.Command("go", "run", "./scripts/stdcorpus", gen)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "XO_"+env+"_SEED="+strconv.Itoa(seed), "XO_STDCORPUS_OUT="+out)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("scripts/stdcorpus %s: %v\n%s", gen, err, b)
	}
	name := gen + "_test.xo"
	text, err := os.ReadFile(filepath.Join(out, name))
	if err != nil {
		t.Fatal(err)
	}
	runFuzzDir(t, name, string(text), envBackends("XO_"+env+"_BACKENDS", []string{"arm64", "llvm"}), strings.Count(string(text), "\n  ("))
}

func TestBrotliFuzz(t *testing.T)       { corpusFuzz(t, "brotli", "BROTLIFUZZ") }
func TestCharsetFuzz(t *testing.T)      { corpusFuzz(t, "charset", "CHARSETFUZZ") }
func TestHtmlparseFuzz(t *testing.T)    { corpusFuzz(t, "htmlparse", "HTMLPARSEFUZZ") }
func TestHtmltokFuzz(t *testing.T)      { corpusFuzz(t, "htmltok", "HTMLTOKFUZZ") }
func TestPublicsuffixFuzz(t *testing.T) { corpusFuzz(t, "publicsuffix", "PUBLICSUFFIXFUZZ") }
func TestRandomFuzz(t *testing.T)       { corpusFuzz(t, "random", "RANDOMFUZZ") }
