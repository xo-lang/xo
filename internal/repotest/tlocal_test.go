package repotest

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// tlocalOwners is who owns each word of the thread local table
// (rt.tlocal_get, rt.tlocal_set, rt.tlocal_addr; std/native/llrt/sched.xo
// documents the table) past the four words every scheduler file shares
// (the current context, the fiber, the worker record, the heap). Word 4
// is pool.xo's scratch on Windows and sanhook.xo's saved errno under
// ThreadSanitizer, which never runs on Windows; 5 and 6 are sysio.xo's
// clock timespec (its offset names word 5). A file that claims a word it
// does not own fails here: tasks.xo once took word 12, sysio.xo's file
// operation message, and every fs error then handed a message string to
// run_claimed as its spare context (gotchas/thread-local-word-collision.md).
var tlocalOwners = map[int][]string{
	4:  {"pool.xo", "sanhook.xo"},
	5:  {"sysio.xo"},
	7:  {"unwind.xo"},
	8:  {"logstate.xo"},
	9:  {"logstate.xo"},
	10: {"libcsys.xo"},
	11: {"sysio.xo"},
	12: {"sysio.xo"},
	13: {"tasks.xo"},
	14: {"libcmem.xo"},
}

var (
	tlocalConst = regexp.MustCompile(`(?m)^(?:pub )?const ([A-Z][A-Z0-9_]*)(?:: Int)? = (\d+)\s*$`)
	tlocalWord  = regexp.MustCompile(`tlocal_(?:get|set)\(\s*([A-Z][A-Z0-9_]*|\d+)`)
	tlocalAt    = regexp.MustCompile(`\b([A-Z][A-Z0-9_]*_AT)\b`)
)

// TestThreadLocalWordOwners resolves every thread local word the native
// runtime touches to the files touching it and checks them against
// tlocalOwners.
func TestThreadLocalWordOwners(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(root, "std", "native", "llrt", "*.xo"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no std/native/llrt sources: %v", err)
	}
	used := map[int]map[string]bool{}
	note := func(w int, f string) {
		if used[w] == nil {
			used[w] = map[string]bool{}
		}
		used[w][f] = true
	}
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
		for _, m := range tlocalWord.FindAllStringSubmatch(src, -1) {
			if v, err := strconv.Atoi(m[1]); err == nil {
				note(v, name)
			} else if v, ok := consts[m[1]]; ok {
				note(v, name)
			} else {
				t.Errorf("%s: thread local word %s is not a constant of the file", name, m[1])
			}
		}
		// byte offsets into the table: on a line with rt.tlocal_addr(),
		// or through a variable holding it (sched.xo's t)
		holders := regexp.MustCompile(`let ([a-z]\w*) = rt\.tlocal_addr\(\)`).FindAllStringSubmatch(src, -1)
		for _, line := range strings.Split(src, "\n") {
			uses := strings.Contains(line, "tlocal_addr()")
			for _, h := range holders {
				if strings.Contains(line, "("+h[1]+", ") {
					uses = true
				}
			}
			if !uses {
				continue
			}
			for _, m := range tlocalAt.FindAllStringSubmatch(line, -1) {
				if v, ok := consts[m[1]]; ok {
					note(v/8, name)
				}
			}
		}
	}
	var words []int
	for w := range used {
		words = append(words, w)
	}
	sort.Ints(words)
	for _, w := range words {
		if w < 4 {
			continue
		}
		if w > 15 {
			t.Errorf("thread local word %d is past the table's 16 words (used by %v)", w, keys(used[w]))
			continue
		}
		owners := tlocalOwners[w]
		for f := range used[w] {
			if !contains(owners, f) {
				t.Errorf("thread local word %d is used by %s, but its owners are %v: pick a free word from std/native/llrt/sched.xo's table, update the table and tlocalOwners", w, f, owners)
			}
		}
	}
}

func keys(m map[string]bool) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}
