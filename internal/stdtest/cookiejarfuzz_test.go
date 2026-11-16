// A fresh seed fuzz run for std/cookiejar against Go's net/http/cookiejar
// (task #99, decision 0616's fuzz kind). The cases are the corpus
// generator's (scripts/stdcorpus/cookiejar.go: seeded scripts of
// SetCookies and Cookies calls over hosts, ports, paths, Domain and Path
// attributes, Secure, Max-Age and Expires, and the public suffix scripts
// psloracle replays with x/net's list), made with XO_COOKIEJARFUZZ_SEED
// added to its seeds and XO_COOKIEJARFUZZ_N scripts (default 400); seed 0
// is the committed testdata/corpus/cookiejar. Xo's jar must send the same
// cookies in the same order after every call. TestCookiejarFuzz runs
// XO_COOKIEJARFUZZ_SEED (default 2) on XO_COOKIEJARFUZZ_BACKENDS (default
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

func TestCookiejarFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the cookiejar fuzz is a heavy test; its seed 0 corpus runs in TestStdCorpora")
	}
	seed := envInt(t, "XO_COOKIEJARFUZZ_SEED", 2)
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	cmd := exec.Command("go", "run", "./scripts/stdcorpus", "cookiejar")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "XO_COOKIEJARFUZZ_SEED="+strconv.Itoa(seed), "XO_COOKIEJARFUZZ_N="+strconv.Itoa(envInt(t, "XO_COOKIEJARFUZZ_N", 400)), "XO_STDCORPUS_OUT="+out)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("scripts/stdcorpus cookiejar: %v\n%s", err, b)
	}
	text, err := os.ReadFile(filepath.Join(out, "cookiejar_test.xo"))
	if err != nil {
		t.Fatal(err)
	}
	runFuzzDir(t, "cookiejar_test.xo", string(text), envBackends("XO_COOKIEJARFUZZ_BACKENDS", []string{"arm64", "llvm"}), envInt(t, "XO_COOKIEJARFUZZ_N", 400))
}
