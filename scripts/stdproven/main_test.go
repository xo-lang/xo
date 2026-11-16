package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
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

func TestRaceRunsStdModuleTests(t *testing.T) {
	root := t.TempDir()
	for _, f := range []string{"std/htmlent.xo", "std/htmlent_test.xo", "std/json.xo", "std/jsonrpc.xo", "std/native/json_test.xo",
		"internal/stdtest/testdata/corpus/json/a.xo", "internal/stdtest/testdata/corpus/jsonfuzz/a.xo", "internal/stdtest/testdata/corpus/jsonrpc/a.xo"} {
		p := filepath.Join(root, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	targets := func(m string) []string {
		var out []string
		for _, r := range raceRuns(root, m) {
			out = append(out, strings.TrimPrefix(r.target, root+string(filepath.Separator)))
		}
		return out
	}
	if got, want := targets("htmlent"), []string{"--std-module=htmlent"}; !reflect.DeepEqual(got, want) {
		t.Errorf("htmlent: %q, want %q", got, want)
	}
	c := filepath.Join("internal", "stdtest", "testdata", "corpus")
	if got, want := targets("json"), []string{"--std-module=native/json", filepath.Join(c, "json"), filepath.Join(c, "jsonfuzz")}; !reflect.DeepEqual(got, want) {
		t.Errorf("json: %q, want %q", got, want)
	}
}

// fuzz shards (#98, lead 2026-11-16: an 8 hour fuzz held host 2 outside
// its lane; one module's 1M cases took 2.5 h and a batch hit offload.sh's
// 180 minutes with no evidence): a shard stops at its minutes with result
// partial, and the next resumes it until the cases or the hours are reached
func TestFuzzShardsMerge(t *testing.T) {
	var clock time.Time
	var seeds []int
	f := fuzzRun{
		hs:    []harness{{"TestXFuzz", "XO_XFUZZ"}},
		cases: 15000, batch: 5000, backends: "arm64,llvm",
		limit: 8 * time.Hour, shard: 20 * time.Minute,
		now: func() time.Time { return clock },
		run: func(h harness, seed, batch int, backends string) ([]byte, error) {
			seeds = append(seeds, seed)
			clock = clock.Add(10 * time.Minute)
			return []byte("--- PASS: " + h.test + " (1s)\n"), nil
		},
	}
	at := time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC)
	st := fuzzResume("", "src1")
	if err := f.loop(&st); err != nil {
		t.Fatal(err)
	}
	one := f.evidence(st, "x", "abc", "src1", "h2", 8, "h2", at)
	for _, want := range []string{"result: partial\n", "seeds: 2 to 3\n", "ran: TestXFuzz=10000\n", "elapsed: 1200\n", "shards: 1\n", "- TestXFuzz seed 3: 5000 cases"} {
		if !strings.Contains(one, want) {
			t.Errorf("shard 1 evidence lacks %q:\n%s", want, one)
		}
	}
	st = fuzzResume(one, "src1")
	if st.first != 2 || st.seed != 4 || st.ran["TestXFuzz"] != 10000 || st.prior != 20*time.Minute || st.shards != 1 || len(st.log) != 2 {
		t.Fatalf("resume: %+v", st)
	}
	if err := f.loop(&st); err != nil {
		t.Fatal(err)
	}
	two := f.evidence(st, "x", "abc", "src1", "h2", 8, "h2", at)
	for _, want := range []string{"result: pass\n", "seeds: 2 to 4\n", "TestXFuzz 15000 cases per backend, 2 backends (arm64,llvm), 30m0s in 2 shards", "shards: 2\n", "- TestXFuzz seed 2:", "- TestXFuzz seed 4:"} {
		if !strings.Contains(two, want) {
			t.Errorf("merged evidence lacks %q:\n%s", want, two)
		}
	}
	if fmt.Sprint(seeds) != "[2 3 4]" {
		t.Errorf("seeds run %v, want [2 3 4]", seeds)
	}
	// the hours count over every shard: a 25 minute run passes in shard 2
	f.limit = 25 * time.Minute
	st = fuzzResume(one, "src1")
	f.loop(&st)
	if !st.pass || st.seed != 5 {
		t.Errorf("a run past its hours over two shards: pass %v seed %d", st.pass, st.seed)
	}
	// another source starts over past the last seed; a pass file only moves the seed
	if st := fuzzResume(one, "src2"); st.first != 4 || st.ran["TestXFuzz"] != 0 || st.prior != 0 || len(st.log) != 0 {
		t.Errorf("another source resumed: %+v", st)
	}
	if st := fuzzResume(two, "src1"); st.first != 5 || st.ran["TestXFuzz"] != 0 || st.shards != 0 {
		t.Errorf("a pass file resumed: %+v", st)
	}
	// stdcov counts result: pass only, so a partial file is no evidence yet
	if strings.Contains(one, "result: pass") {
		t.Error("a partial shard says pass")
	}
}
