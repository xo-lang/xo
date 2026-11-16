package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestSourceHashStopsAtRuntime: decision 0616 section 3 (amended
// 2026-11-14). A module that uses no other std module (deque; uuid used
// none until its v3 and v5 used crypto, 2026-11-15) hashes its own file
// alone (std/internal.xo too when it uses it), and no module's hash
// reaches the runtime core or the prelude.
func TestSourceHashStopsAtRuntime(t *testing.T) {
	if err := os.Chdir("../.."); err != nil {
		t.Fatal(err)
	}
	_, files, err := sourceHash("deque")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"std/deque.xo"}; !reflect.DeepEqual(files, want) {
		t.Fatalf("deque hashes %v, want %v", files, want)
	}
	rt, err := runtimeFiles()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range []string{"time", "json", "url", "http"} {
		_, files, err := sourceHash(m)
		if err != nil {
			t.Fatal(err)
		}
		own := false
		for _, f := range files {
			if rt[f] || strings.HasPrefix(filepath.Base(f), "sys_") {
				t.Errorf("%s hashes runtime file %s", m, f)
			}
			own = own || f == filepath.Join("std", m+".xo")
		}
		if !own {
			t.Errorf("%s does not hash std/%s.xo: %v", m, m, files)
		}
	}
	// url hashes its native half too; the modules it imports (netip since
	// the URL type, decision 0642) may change, so present, not exact
	_, files, _ = sourceHash("url")
	has := map[string]bool{}
	for _, f := range files {
		has[f] = true
		if rt[f] {
			t.Errorf("url hashes runtime file %s", f)
		}
	}
	if !has["std/native/url_native.xo"] || !has["std/url.xo"] {
		t.Errorf("url hashes %v, want std/url.xo and its std/native/url_native.xo", files)
	}
}

// TestPreludeHash: the prelude's source hash covers the runtime it is the
// entry of (the other modules' hashes stop there).
func TestPreludeHash(t *testing.T) {
	if _, err := os.Stat("std/prelude.xo"); err != nil {
		// run alone: the package directory, not the root an earlier test moved to
		if err := os.Chdir("../.."); err != nil {
			t.Fatal(err)
		}
	}
	_, files, err := sourceHash("prelude")
	if err != nil {
		t.Fatal(err)
	}
	has := map[string]bool{}
	for _, f := range files {
		has[filepath.ToSlash(f)] = true
	}
	for _, want := range []string{"std/prelude.xo", "std/native/prelude.xo", "std/native/rt.xo", "std/native/llrt/alloc.xo"} {
		if !has[want] {
			t.Errorf("prelude hash files %d, missing %s", len(files), want)
		}
	}
}

// TestPerfOwner: a bench/perf directory is the longest std module's its
// name starts with, or the one perfOwners names.
func TestPerfOwner(t *testing.T) {
	for dir, want := range map[string]string{"html": "html", "htmlparse": "htmlparse", "htmltok": "htmltok", "jsontoken": "json", "timeformat-cache": "time", "url": "url", "aesgcm": "crypto", "ed25519": "crypto", "sort": "prelude", "strsplit": "prelude", "bytessearch": "prelude"} {
		if got := perfOwner(dir); got != want {
			t.Errorf("perfOwner(%q) = %q, want %q", dir, got, want)
		}
	}
}

// TestSummaryLine: a stress program that returns Err prints its summary,
// then main's "error: ..." line; the race kind judges the summary.
func TestSummaryLine(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"a\ncrc32 stress: 1 operations, 0 mismatches, rss 1 MB, FAIL (memory grew)\nerror: memory grew 12.6%", "crc32 stress: 1 operations, 0 mismatches, rss 1 MB, FAIL (memory grew)"},
		{"x stress: ok, pass", "x stress: ok, pass"},
		{"error: only", "error: only"},
	} {
		if got := summaryLine(c.in); got != c.want {
			t.Errorf("summaryLine(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestKilledUnchecked(t *testing.T) {
	for _, c := range []struct {
		out  string
		want bool
	}{
		{"    idnafuzz_test.go:34: xo test --backend=llvm: signal: killed; 0 passed, 0 failed; the generated file holds 1 tests (40000 generated cases)", true},
		{"    idnafuzz_test.go:34: xo test --backend=llvm: exit status 1; 0 passed, 1 failed; the generated file holds 1 tests", false},
		{"    idnafuzz_test.go:34: xo test --backend=arm64: signal: killed; 3 passed, 0 failed", false},
	} {
		if got := killedUnchecked(c.out); got != c.want {
			t.Errorf("killedUnchecked(%q) = %v, want %v", c.out, got, c.want)
		}
	}
}

func TestNotRun(t *testing.T) {
	for _, c := range []struct{ out, want string }{
		{"=== RUN   TestYamlFuzz\n    yamlfuzz_test.go:34: yamloracle cannot run\n--- SKIP: TestYamlFuzz (0.10s)\nPASS\nok  \tx/internal/stdtest\t0.2s", "skipped"},
		{"testing: warning: no tests to run\nPASS\nok  \tx/internal/stdtest\t0.1s [no tests to run]", "did not run"},
		{"PASS\nok  \tx/internal/stdtest\t0.1s", "did not run"},
		{"=== RUN   TestYamlFuzz\n--- PASS: TestYamlFuzz (12.00s)\nPASS\nok  \tx/internal/stdtest\t12.1s", ""},
	} {
		if got := notRun("TestYamlFuzz", c.out); got != c.want {
			t.Errorf("notRun(%q) = %q, want %q", c.out, got, c.want)
		}
	}
}
