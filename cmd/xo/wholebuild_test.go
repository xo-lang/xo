package main

import (
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/testtmp"
)

// wholeWarnSrc prints "one" and draws warning XO0540, which a hit must
// print again.
const wholeWarnSrc = `fn bump(m: Mutex[Int]) -> () ! Str {
  m.with(fn(var n) -> () ! Str {
    n = n + 1
    Err("no")
  })
}

fn main(os: Os) {
  let m = Mutex[Int].new(0)
  let _r = bump(m).ok()
  os.stdio.println("one")
}

test "one" {
  expect(1 + 1) == 2
}
`

// TestWholeBuildCache: by default `xo build` and `xo run` use the run
// cache of `xo test` (decision 0360 stage 1): an unchanged
// program's binary is copied to the output (or run) with what its build
// printed, its warning included, and every input change misses: an edit,
// a new file, an XO_ variable, a flag, an xo.work above the module (a
// path the build only looked for, stage 2's recorder), and a std file
// edited at the same size with its modification time put back.
// XO_BACKEND_CACHE=verify rebuilds a hit: a good entry passes, a planted
// wrong binary is XO1512. A file touched but unchanged
// still hits (the entry hashes contents). Two builds of one output at
// once leave a whole binary. `xo test` of a program with a warning hits
// and prints the warning (such a build was never stored before).
func TestWholeBuildCache(t *testing.T) {
	if testing.Short() {
		t.Skip("builds programs")
	}
	tmp := t.TempDir()
	setenv(t, "XO_BUILD_DIR", filepath.Join(tmp, "build"))
	setenv(t, "XO_BIN_CACHE", filepath.Join(tmp, "bin"))
	setenv(t, "XO_BACKEND_CACHE_DIR", filepath.Join(tmp, "backend"))
	// a copy of std, so a std file can be edited
	std := filepath.Join(tmp, "std")
	if err := os.CopyFS(std, os.DirFS(filepath.Join(repoRoot, "std"))); err != nil {
		t.Fatal(err)
	}
	setenv(t, "XO_STD", std)
	// the default, named so that switching it off and back keeps one key
	// (the key holds every XO_ variable)
	setenv(t, "XO_WHOLE_BUILD", "on")
	dir := filepath.Join(tmp, "prog")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("xo.mod", "module whole\n")
	write("main.xo", wholeWarnSrc)
	chdir(t, dir)
	out := filepath.Join(dir, "prog")
	// one command: its exit code, standard output, standard error without
	// the --phase-times reports, and whether the run cache served it
	xo := func(args ...string) (int, string, string, bool) {
		t.Helper()
		var so, se bytes.Buffer
		code := run(t, append(append([]string{}, args[:1]...), append([]string{"--phase-times"}, args[1:]...)...), &so, &se)
		var kept []string
		hit := false
		for _, l := range strings.SplitAfter(se.String(), "\n") {
			if strings.HasPrefix(l, "phase run_cache ") {
				hit = true
			}
			// xo's own reports, which a hit does not print again
			if !strings.HasPrefix(l, "phase ") && !strings.HasPrefix(l, "cache ") && !strings.HasPrefix(l, "fe ") {
				kept = append(kept, l)
			}
		}
		return code, so.String(), strings.Join(kept, ""), hit
	}
	ran := func(what, want string) {
		t.Helper()
		got, err := testtmp.Guard(exec.Command(out)).Output()
		if err != nil || string(got) != want {
			t.Fatalf("%s: the binary printed %q (%v), want %q", what, got, err, want)
		}
	}
	build := func(what string, wantHit bool, args ...string) (string, string) {
		t.Helper()
		code, so, se, hit := xo(append([]string{"build", "-o", out}, append(args, ".")...)...)
		if code != 0 || hit != wantHit || !strings.Contains(se, "warning XO0540") {
			t.Fatalf("%s: exit %d, cache hit %v (want %v), want warning XO0540 in\n%s%s", what, code, hit, wantHit, so, se)
		}
		return so, se
	}
	so1, se1 := build("first build", false)
	ran("first build", "one\n")
	// XO_WHOLE_BUILD=off: the entry is not used
	setenv(t, "XO_WHOLE_BUILD", "off")
	build("XO_WHOLE_BUILD=off", false)
	setenv(t, "XO_WHOLE_BUILD", "on")
	so2, se2 := build("unchanged", true)
	if so2 != so1 || se2 != se1 {
		t.Fatalf("a hit printed otherwise than the build\n--- build\n%s%s--- hit\n%s%s", so1, se1, so2, se2)
	}
	ran("unchanged", "one\n")
	later := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(filepath.Join(dir, "main.xo"), later, later); err != nil {
		t.Fatal(err)
	}
	build("touched, unchanged", true)
	write("main.xo", strings.Replace(wholeWarnSrc, `"one"`, `"two"`, 1))
	build("edited", false)
	ran("edited", "two\n")
	build("edited, unchanged", true)
	write("more.xo", "fn more() -> Int { 1 }\n")
	build("new file", false)
	build("new file, unchanged", true)
	setenv(t, "XO_INLINE_DENY", "no_such_function")
	build("an XO_ variable", false)
	build("an XO_ variable, unchanged", true)
	build("a flag", false, "--error-args=off")
	build("a flag, unchanged", true, "--error-args=off")
	// an xo.work above the module: the build looked for one and found
	// none, so finding one now misses
	if err := os.WriteFile(filepath.Join(tmp, "xo.work"), []byte("xo preview\nuse ./prog\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	build("an xo.work above", false, "--error-args=off")
	build("an xo.work above, unchanged", true, "--error-args=off")

	// XO_BACKEND_CACHE=verify rebuilds a hit and compares: equal passes;
	// a planted wrong binary in the entry is XO1512
	setenv(t, "XO_BACKEND_CACHE", "verify")
	build("verify, a good entry", false, "--error-args=off")
	bins, _ := filepath.Glob(filepath.Join(tmp, "bin", "run", "*", "*.bin"))
	if len(bins) == 0 {
		t.Fatal("no run cache entry to plant")
	}
	for _, e := range bins {
		if err := os.WriteFile(e, []byte("not the program"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if code, so, se, _ := xo("build", "-o", out, "--error-args=off", "."); code != 1 || !strings.Contains(se, "XO1512") {
		t.Fatalf("verify, a planted entry: exit %d, want 1 and XO1512\n%s%s", code, so, se)
	}
	setenv(t, "XO_BACKEND_CACHE", "on")

	// a std file edited at the same size, its modification time put back
	// (touch -r): the contents are hashed, so it misses
	var target string
	filepath.WalkDir(std, func(p string, d fs.DirEntry, err error) error {
		if err == nil && target == "" && !d.IsDir() && strings.HasSuffix(p, ".md") {
			target = p
		}
		return nil
	})
	fi, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(target)
	if err != nil || len(b) == 0 {
		t.Fatalf("read %s: %v", target, err)
	}
	b[0] ^= 0x20
	if err := os.WriteFile(target, b, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(target, fi.ModTime(), fi.ModTime()); err != nil {
		t.Fatal(err)
	}
	build("a std file edited, its time put back", false, "--error-args=off")

	// two builds of one output at once, missing then hitting: the output
	// is a whole binary each time
	write("main.xo", strings.Replace(wholeWarnSrc, `"one"`, `"three"`, 1))
	for _, round := range []string{"missing", "hitting"} {
		var wg sync.WaitGroup
		codes := make([]int, 2)
		for i := range codes {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				codes[i], _, _, _ = xo("build", "-o", out, ".")
			}(i)
		}
		wg.Wait()
		if codes[0] != 0 || codes[1] != 0 {
			t.Fatalf("two builds at once, %s: exits %v", round, codes)
		}
		ran("two builds at once, "+round, "three\n")
	}

	// xo run: the program runs, the warning printed again
	for _, wantHit := range []bool{false, true} {
		code, so, se, hit := xo("run", ".")
		if code != 0 || so != "three\n" || hit != wantHit || !strings.Contains(se, "warning XO0540") {
			t.Fatalf("xo run: exit %d, cache hit %v (want %v), output %q\n%s", code, hit, wantHit, so, se)
		}
	}

	// xo test of a program with a warning: stored, then a hit that prints it
	for _, wantHit := range []bool{false, true} {
		code, so, se, hit := xo("test", ".")
		if code != 0 || hit != wantHit || !strings.Contains(so+se, "1 passed") || !strings.Contains(se, "warning XO0540") {
			t.Fatalf("xo test: exit %d, cache hit %v (want %v)\n%s%s", code, hit, wantHit, so, se)
		}
	}
}
