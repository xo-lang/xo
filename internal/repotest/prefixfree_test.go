package repotest

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// handPrefix matches a call of rt.heap_release or rt.heap_large_resize
// whose first argument is an address minus 8 written out: `x - 8`,
// `x.wrapping_sub(8)` or `subw(x, 8)`.
var handPrefix = regexp.MustCompile(`rt\.heap_(?:release|large_resize)\((?:[^,()]|\([^()]*\))*?(?:-\s*8\b|\.wrapping_sub\(8\)|subw\([^()]*,\s*8\))`)

// TestNoHandPrefixFree checks that std/native gives back a block
// rt.heap_large returned through std/native/llrt/heapblk.xo (release and
// resize, which take the returned address and do the `- 8`), never a
// hand `rt.heap_release(x - 8)`: five fixes (94d73568b, 5d3d1380a,
// 30c685590, ab1d601c0, 652514dcc) were such a `- 8` done wrong or left
// out (task #354, class guard 5 of 2026-11-18). A variable that holds the
// prefix itself is not matched.
func TestNoHandPrefixFree(t *testing.T) {
	dir := filepath.Join(root, "std", "native")
	n := 0
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".xo") || strings.HasSuffix(p, "_test.xo") || filepath.Base(p) == "heapblk.xo" {
			return nil
		}
		n++
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue
			}
			if m := handPrefix.FindString(line); m != "" {
				rel, _ := filepath.Rel(root, p)
				t.Errorf("%s:%d: %s...: give the address rt.heap_large returned to heapblk.release or heapblk.resize (std/native/llrt/heapblk.xo)", rel, i+1, m)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatalf("no sources under %s", dir)
	}
}
