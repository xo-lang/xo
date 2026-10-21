package llvm

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

// TestTestSplitReuse: an LLVM test build splits the program's IR by
// function name into modules whose objects the back-end cache keeps
// (XO_LLVM_TEST_SPLIT), so after an edit to one function's body most
// modules are taken from the cache, and the tests still pass.
func TestTestSplitReuse(t *testing.T) {
	if testing.Short() {
		t.Skip("builds test programs with clang")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	own := filepath.Join(work, "p")
	if err := os.MkdirAll(own, 0o755); err != nil {
		t.Fatal(err)
	}
	// the edit adds a line to more's body, the last function: only the
	// module holding more changes (each module carries only the debug
	// metadata of its own functions)
	lib := func(k int) string {
		extra := ""
		if k > 1 {
			extra = "  let _y = " + strconv.Itoa(k) + "\n"
		}
		return "pub fn twice(x: Int) -> Int {\n  x * 2\n}\n\npub fn more(x: Int) -> Int {\n" + extra + "  x + " + strconv.Itoa(k) + "\n}\n"
	}
	files := map[string]string{
		"xo.mod":      "module example.com/p\n",
		"lib.xo":      lib(1),
		"lib_test.xo": "test \"twice\" {\n  expect(twice(2)) == 4\n}\n\ntest \"more\" {\n  expect(more(2) > 2) == true\n}\n",
	}
	for p, s := range files {
		if err := os.WriteFile(filepath.Join(own, p), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cache := filepath.Join(work, "cache")
	note := regexp.MustCompile(`cache llvm modules (\d+)/(\d+) reused`)
	run := func() (int, int) {
		t.Helper()
		cmd := exec.Command(xo, "test", "--backend=llvm", "--phase-times", ".")
		cmd.Dir = own
		cmd.Env = append(os.Environ(), "XO_BACKEND_CACHE_DIR="+cache, "XO_BACKEND_CACHE=on", "XO_BIN_CACHE="+filepath.Join(work, "bin"))
		log, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("xo test: %v\n%s", err, log)
		}
		m := note.FindSubmatch(log)
		if m == nil {
			t.Fatalf("no `cache llvm modules` line:\n%s", log)
		}
		r, _ := strconv.Atoi(string(m[1]))
		n, _ := strconv.Atoi(string(m[2]))
		return r, n
	}
	if r, n := run(); r != 0 || n < 2 {
		t.Fatalf("cold build: %d/%d modules reused, want 0 of at least 2", r, n)
	}
	if err := os.WriteFile(filepath.Join(own, "lib.xo"), []byte(lib(3)), 0o644); err != nil {
		t.Fatal(err)
	}
	r, n := run()
	if r < n-2 {
		t.Errorf("after an edit of one body: %d/%d modules reused, want all but one or two", r, n)
	}
}
