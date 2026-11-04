package repotest

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// heapListWord is the static word holding the heap record list's head
// (std/native/llrt/heapos.xo W_HEAPS).
const heapListWord = 27

var (
	staticWordUse = regexp.MustCompile(`static_word\(\s*([A-Z][A-Z0-9_]*|\d+)\s*\)`)
	xoFuncHead    = regexp.MustCompile(`^(?:pub )?fn (?:\([^)]*\) )?([a-z_][a-z0-9_]*)\s*[(\[]`)
)

// TestHeapListHeadAccessors checks that the heap record list's head
// (static word W_HEAPS) is read and written only by heapos.xo's
// heaps_head (an acquire load) and heaps_set (a release store): a pool
// thread pushes its record there while other threads read it, and each
// plain load added later was a ThreadSanitizer race (three times,
// 2026-11-02 and 2026-11-03: gotchas/heap-list-head-is-newest-record.md).
func TestHeapListHeadAccessors(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(root, "std", "native", "llrt", "*.xo"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no std/native/llrt sources: %v", err)
	}
	allowed := map[string]bool{"heaps_head": true, "heaps_set": true}
	seen := map[string]bool{}
	for _, p := range files {
		if strings.HasSuffix(p, "_test.xo") {
			continue
		}
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		src := string(b)
		name := filepath.Base(p)
		consts := map[string]int{}
		for _, m := range tlocalConst.FindAllStringSubmatch(src, -1) {
			v, _ := strconv.Atoi(m[2])
			consts[m[1]] = v
		}
		fn := ""
		for i, line := range strings.Split(src, "\n") {
			if m := xoFuncHead.FindStringSubmatch(line); m != nil {
				fn = m[1]
			}
			for _, m := range staticWordUse.FindAllStringSubmatch(line, -1) {
				v, err := strconv.Atoi(m[1])
				if err != nil {
					c, ok := consts[m[1]]
					if !ok {
						continue
					}
					v = c
				}
				if v != heapListWord {
					continue
				}
				if name == "heapos.xo" && allowed[fn] {
					seen[fn] = true
					continue
				}
				t.Errorf("%s:%d: %s touches the heap list head (static word %d) directly; use heapos.heaps_head() (acquire) or heaps_set (release)", name, i+1, fn, heapListWord)
			}
		}
	}
	for f := range allowed {
		if !seen[f] {
			t.Errorf("heapos.xo has no %s reading static word %d: the accessor moved or the word changed; update TestHeapListHeadAccessors", f, heapListWord)
		}
	}
}
