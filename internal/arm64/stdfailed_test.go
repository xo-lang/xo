package arm64

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestStdFailedMarkerPerTree is b26 of the owner's v0.38.1 evaluation:
// std's store key follows std's relative paths, sizes and times, not its
// place, so two copies of one std share it. A failed store build of one
// copy (here: its zoneinfo.zip removed, as a deleted release smoke test's
// was) must not turn separate compilation off for the other, and a
// failure marker past its hour is tried again.
func TestStdFailedMarkerPerTree(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds programs")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-stdfailed-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	a := filepath.Join(work, "a", "std")
	b := filepath.Join(work, "b", "std")
	for _, d := range []string{a, b} {
		copyTreeTimes(t, filepath.Join(root, "std"), d)
	}
	if err := os.Remove(filepath.Join(a, "tzdata", "zoneinfo.zip")); err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(work, "cache")
	home := filepath.Join(work, "home")
	prog := filepath.Join(work, "prog")
	for _, d := range []string{home, prog} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(prog, "main.xo"), []byte("fn main(os: Os) {\n  os.stdio.println(\"hi\")\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(prog, "xo.mod"), []byte("module example.com/p\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	env := func(std string) []string {
		return append(os.Environ(), "HOME="+home, "XO_BACKEND_CACHE=on", "XO_BACKEND_CACHE_DIR="+cache, "XO_STD="+std, "XO_SEPCOMP_BG=0", "XO_BIN_CACHE=off")
	}
	fail := func() string {
		c := testtmp.Guard(exec.Command(xo, "dev", "stdcache", "--conc=0"))
		c.Env = env(a)
		if out, err := c.CombinedOutput(); err == nil {
			t.Fatalf("xo dev stdcache of the std without zoneinfo.zip succeeded:\n%s", out)
		}
		ms, _ := filepath.Glob(filepath.Join(cache, "std", "*.store.failed"))
		if len(ms) != 1 {
			t.Fatalf("failure markers %q, want one", ms)
		}
		body, err := os.ReadFile(ms[0])
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), "\nstd "+a+"\n") {
			t.Fatalf("marker %s does not name the failed std %s:\n%s", ms[0], a, body)
		}
		return ms[0]
	}
	build := func(std string) string {
		c := testtmp.Guard(exec.Command(xo, "build", "-o", filepath.Join(work, "out"), "."))
		c.Dir = prog
		c.Env = env(std)
		out, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("xo build with %s: %v\n%s", std, err, out)
		}
		return string(out)
	}
	const off = "separate compilation is off"
	marker := fail()
	// the other copy: no warning, and the marker it does not hold for is gone
	if out := build(b); strings.Contains(out, off) {
		t.Fatalf("the intact std's build warned:\n%s", out)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("marker %s of another std still there after the intact std's build (%v)", marker, err)
	}
	// the copy that failed: the warning, naming the marker
	marker = fail()
	if out := build(a); !strings.Contains(out, off) || !strings.Contains(out, marker) {
		t.Fatalf("the failed std's build did not warn with its marker %s:\n%s", marker, out)
	}
	// past its hour the marker is tried again (the build goes on whole
	// program in the background's absence, with no warning)
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(marker, old, old); err != nil {
		t.Fatal(err)
	}
	if out := build(a); strings.Contains(out, off) {
		t.Fatalf("a marker two hours old still warned:\n%s", out)
	}
}

// copyTreeTimes copies the tree src to dst with each file's modification
// time, as tar and cp -p do (std's key holds the times).
func copyTreeTimes(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		to := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(to, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if err := os.WriteFile(to, data, 0o644); err != nil {
			return err
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		return os.Chtimes(to, fi.ModTime(), fi.ModTime())
	})
	if err != nil {
		t.Fatal(err)
	}
}
