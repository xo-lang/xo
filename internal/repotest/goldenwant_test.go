package repotest

import (
	"bytes"
	"os/exec"
	"path"
	"strings"
	"testing"
)

// TestGoldensHaveWant: every program golden (a directory with a main.xo
// under internal/gen or internal/llvm testdata/golden) has its want.txt.
// dbcaeaf97 added internal/gen's test_fakenet_tls without one; the push
// passed, and the 2026-11-07 09:06 main check failed on "missing want.txt".
// internal/check (want.json) and internal/ssa keep other layouts.
func TestGoldensHaveWant(t *testing.T) {
	out, err := exec.Command("git", "-C", root, "ls-files", "-z", "--",
		"internal/gen/testdata/golden", "internal/llvm/testdata/golden").Output()
	if err != nil {
		t.Skipf("git ls-files: %v", err)
	}
	mains := map[string]bool{}
	wants := map[string]bool{}
	for _, name := range bytes.Split(out, []byte{0}) {
		n := string(name)
		dir, file := path.Split(n)
		dir = strings.TrimSuffix(dir, "/")
		if path.Dir(dir) != "internal/gen/testdata/golden" && path.Dir(dir) != "internal/llvm/testdata/golden" {
			continue
		}
		switch file {
		case "main.xo":
			mains[dir] = true
		case "want.txt":
			wants[dir] = true
		}
	}
	for dir := range mains {
		if !wants[dir] {
			t.Errorf("%s has main.xo but no want.txt (xo dev new-golden writes the exit/stdout/stderr layout)", dir)
		}
	}
}
