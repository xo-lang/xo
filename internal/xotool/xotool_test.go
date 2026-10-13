package xotool

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	r, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// TestInputs: the key covers selfhost/ and the std modules it reaches
// (the prelude, std/json through selfhost's `use` lines), not the C
// runtime or third_party, which xo reads at run time.
func TestInputs(t *testing.T) {
	root := repoRoot(t)
	in, err := Inputs(root)
	if err != nil {
		t.Fatal(err)
	}
	has := map[string]bool{}
	for _, n := range in {
		has[n] = true
		if strings.HasPrefix(n, "std/llrt/") || strings.HasPrefix(n, "third_party/") || strings.HasSuffix(n, ".go") {
			t.Errorf("%s is an input", n)
		}
	}
	for _, n := range []string{"selfhost/xo/main.xo", "selfhost/xo.mod", "std/prelude.xo", "std/native/prelude.xo"} {
		if !has[n] {
			t.Errorf("%s is not an input", n)
		}
	}
	k1, err := Key(root)
	if err != nil {
		t.Fatal(err)
	}
	k2, _ := Key(root)
	if k1 != k2 || len(k1) != 16 {
		t.Errorf("keys %q %q", k1, k2)
	}
}

// TestIOFlagsMatchXo requires IOFlags to name the runtime symbols xo's
// own table does (selfhost/xo/link.xo io_flags), so the runtime tests
// compile the runtime as xo does (gotchas/xo1-duplicated-tables-drift.md).
func TestIOFlagsMatchXo(t *testing.T) {
	goSyms := bodySymbols(t, "cc.go", "func IOFlags(", "\n}\n")
	xoSyms := bodySymbols(t, filepath.Join(repoRoot(t), "selfhost", "xo", "link.xo"), "fn io_flags(", "\n}\n")
	if len(goSyms) == 0 || len(xoSyms) == 0 {
		t.Fatalf("no symbols found: %d, %d", len(goSyms), len(xoSyms))
	}
	if strings.Join(goSyms, " ") != strings.Join(xoSyms, " ") {
		t.Errorf("IOFlags differs from xo's io_flags:\n Go: %v\n Xo: %v", goSyms, xoSyms)
	}
}

var runtimeSym = regexp.MustCompile(`"(xo_[a-z0-9_]+)"`)

// bodySymbols returns the sorted xo_ symbols quoted in the function of
// file that starts at start and ends at the first end after it.
func bodySymbols(t *testing.T, file, start, end string) []string {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	i := strings.Index(s, start)
	if i < 0 {
		t.Fatalf("%s: %q not found", file, start)
	}
	s = s[i:]
	if j := strings.Index(s, end); j >= 0 {
		s = s[:j]
	}
	var out []string
	for _, m := range runtimeSym.FindAllStringSubmatch(s, -1) {
		out = append(out, m[1])
	}
	sort.Strings(out)
	return out
}
