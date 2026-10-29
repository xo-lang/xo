package arm64

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestStdRecordIdentity: a separately compiled build that writes the std
// record (decision 0388 stage 2), one that loads it, and one without the
// record (XO_STD_EXPORT=0) give the same bytes. The build info's "<std>"
// entry hashed empty sources for the files a record made and left
// unparsed, so the first build of a program had another build id than
// every later one (Windows run 40820335875, 130 bytes; darwin 190,
// 2026-10-29). One output name for the three builds: the code signature
// identifier is the file's name.
func TestStdRecordIdentity(t *testing.T) {
	if testing.Short() {
		t.Skip("builds programs")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-stdrecord-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	cache := filepath.Join(work, "cache")
	cmd := testtmp.Guard(exec.Command(xo, "dev", "stdcache", "--conc=0"))
	cmd.Env = append(os.Environ(), "XO_BACKEND_CACHE=on", "XO_BACKEND_CACHE_DIR="+cache)
	paths, err := cmd.Output()
	if err != nil {
		t.Fatalf("xo dev stdcache: %v", err)
	}
	lines := strings.Fields(string(paths))
	if len(lines) != 2 {
		t.Fatalf("xo dev stdcache printed %q, want the store and the facts", paths)
	}
	store, facts := lines[0], lines[1]
	// std's split, to_upper and a format: the runtime's files under the
	// std directory, which the "<std>" entry hashes
	src := `fn sum_to(n: Int) -> Int {
  if n <= 0 {
    return 0
  }
  let rest = sum_to(n - 1)
  n + rest
}

fn main(os: Os) {
  let words = "alpha,beta,gamma".split(",")
  var total = 0
  for w in words {
    total += w.to_upper().len()
  }
  os.stdio.println("sum ${sum_to(5)} words ${words.len()} total ${total}")
}
`
	prog := filepath.Join(work, "prog")
	if err := os.MkdirAll(prog, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(prog, "main.xo"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(prog, "xo.mod"), []byte("module prog\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(), "XO_BACKEND_CACHE=on", "XO_BACKEND_CACHE_DIR="+cache, "XO_CACHE_STATS=1",
		"XO_SEPCOMP=1", "XO_SEPCOMP_FACTS="+facts, "XO_SEPCOMP_STORE="+store)
	build := func(name string, extra ...string) ([]byte, string) {
		dir := filepath.Join(work, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		out := filepath.Join(dir, "prog")
		b := testtmp.Guard(exec.Command(xo, "build", "-o", out, "."))
		b.Dir = prog
		b.Env = append(append([]string{}, env...), extra...)
		msgs, err := b.CombinedOutput()
		if err != nil {
			t.Fatalf("build %s: %v\n%s", name, err, msgs)
		}
		bin, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		return bin, string(msgs)
	}
	// xo dev stdcache wrote a record of its own: the first program build
	// must be the writer, so the records go first
	if err := os.RemoveAll(filepath.Join(cache, "stdx")); err != nil {
		t.Fatal(err)
	}
	writer, wmsgs := build("writer", "XO_STD_EXPORT=1")
	// "cache stdx no entry" is the lookup's line; the "declared" line is
	// printed with --phase-times only
	if !strings.Contains(wmsgs, "cache stdx no entry") {
		t.Fatalf("the first build did not write the record:\n%s", wmsgs)
	}
	loader, lmsgs := build("loader", "XO_STD_EXPORT=1")
	if !strings.Contains(lmsgs, "cache stdx record loaded") {
		t.Fatalf("the second build did not load the record:\n%s", lmsgs)
	}
	plain, _ := build("plain", "XO_STD_EXPORT=0")
	differing := func(a, b []byte) int {
		n := 0
		for i := 0; i < len(a) && i < len(b); i++ {
			if a[i] != b[i] {
				n++
			}
		}
		if len(a) != len(b) {
			n += len(a) - len(b)
			if n < 0 {
				n = -n
			}
		}
		return n
	}
	if !bytes.Equal(writer, loader) {
		t.Errorf("the build that wrote the record and the one that loaded it differ in %d bytes", differing(writer, loader))
	}
	if !bytes.Equal(writer, plain) {
		t.Errorf("the build that wrote the record and the build without it differ in %d bytes", differing(writer, plain))
	}
}
