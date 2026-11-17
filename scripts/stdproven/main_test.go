package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
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
// TestRaceMemOnly: a stress program under --race that failed on memory
// alone, with every result right, is judged by the task churn, whatever
// its summary calls the count (2026-11-17: dns's "0 wrong answers" failed
// its race evidence on memory).
func TestRaceMemOnly(t *testing.T) {
	for _, c := range []struct {
		line string
		want bool
	}{
		{"bufio stress: 9 operations by 64 tasks in 10 min, 0 mismatches, rss 1 MB, FAIL (memory grew)", true},
		{"dns stress: 39309508 lookups by 64 clients in 10 min, 0 wrong answers, rss 204 MB at the mark, 273 MB at the end (34.0%), peak 273 MB, FAIL (memory grew)", true},
		{"smtp stress: 5 sends by 64 clients in 10 min (1 MB of bodies taken), 0 wrong results, rss 3 MB, FAIL (memory grew)", true},
		{"bufio stress: 9 operations by 64 tasks in 10 min, 10 mismatches, rss 1 MB, FAIL (memory grew)", false},
		{"jsonrpc stress: 3703204 calls by 128 tasks in 10 min, 231461 notifications, every answer right, rss 1223 MB at the mark, 2223 MB at the end (82.0%), peak 2223 MB, FAIL (memory grew)", true},
		{"asn1 stress: 9 values by 64 tasks in 10 min, 0 identities broken, rss 1 MB, FAIL (memory grew)", true},
		{"amqptest stress: 9 messages by 64 producers, 9 accepted once and 0 dead lettered by 64 consumers in 10 min, none lost, rss 1 MB, FAIL (memory grew)", true},
		{"otlp stress: 9 exports by 64 tasks in 10 min (9 traces and 9 metrics requests taken), 0 wrong, rss 1 MB, FAIL (memory grew)", true},
		{"multipart stress: 9 forms by 64 tasks in 10 min (1 MB written and read back 3 ways), 0 differences, rss 1 MB, FAIL (memory grew)", true},
		{"sse stress: 9 events by 64 subscribers in 10 min over 9 streams, every event once and in order, rss 1 MB, FAIL (memory grew)", true},
		{"trace stress: 9 spans recorded and 9 read back by 64 tasks in 10 min, 0 promises broken, rss 1 MB, FAIL (memory grew)", true},
		{"asn1 stress: 9 values by 64 tasks in 10 min, 2 identities broken, rss 1 MB, FAIL (memory grew)", false},
		{"otlp stress: 9 exports by 64 tasks in 10 min, 10 wrong, rss 1 MB, FAIL (memory grew)", false},
		{"dns stress: 9 lookups by 64 clients in 10 min, 3 wrong answers, rss 1 MB, FAIL (memory grew)", false},
		{"dns stress: 9 lookups by 64 clients in 10 min, 0 wrong answers, rss 1 MB, pass", false},
	} {
		if got := raceMemOnly(c.line); got != c.want {
			t.Errorf("raceMemOnly(%q) = %v, want %v", c.line, got, c.want)
		}
	}
}

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
	for _, f := range []string{"std/htmlent.xo", "std/htmlent_test.xo", "std/cbor_test.xo", "std/native/cbor_test.xo", "std/json.xo", "std/jsonrpc.xo", "std/native/json_test.xo",
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
	if got, want := targets("htmlent"), []string{"--std-module=std/htmlent"}; !reflect.DeepEqual(got, want) {
		t.Errorf("htmlent: %q, want %q", got, want)
	}
	// both std/native/cbor and std/cbor: a bare cbor is ambiguous to xo
	// test (#152: cbor's race failed in 5 s on host 4)
	if got, want := targets("cbor"), []string{"--std-module=native/cbor", "--std-module=std/cbor"}; !reflect.DeepEqual(got, want) {
		t.Errorf("cbor: %q, want %q", got, want)
	}
	c := filepath.Join("internal", "stdtest", "testdata", "corpus")
	if got, want := targets("json"), []string{"--std-module=native/json", filepath.Join(c, "json"), filepath.Join(c, "jsonfuzz")}; !reflect.DeepEqual(got, want) {
		t.Errorf("json: %q, want %q", got, want)
	}
}

// A module whose tests use net runs them with --real: the fake network has
// no UDP, so std/udp's tests failed under stdproven race (2026-11-16).
func TestRaceRunsRealNet(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"std/udp_test.xo":  "use std/task\n\ntest \"a datagram\" uses (net, clock) {\n}\n",
		"std/text_test.xo": "test \"plain\" {\n}\n// uses (net) in a comment\n",
	}
	for f, body := range files {
		p := filepath.Join(root, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for m, want := range map[string]bool{"udp": true, "text": false} {
		runs := raceRuns(root, m)
		if len(runs) != 1 || runs[0].real != want {
			t.Errorf("%s: runs %+v, want one with real=%v", m, runs, want)
		}
	}
}

func TestRaceStressOnly(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "std", "evidence", "websocket", "stress", "main.xo")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		module    string
		runs, min int
		want      bool
		err       string
	}{
		{"websocket", 0, 10, true, ""},
		{"websocket", 2, 10, false, ""},
		{"websocket", 0, 0, false, "-stress-minutes 0 skips"},
		{"dns", 0, 10, false, "no stress program"},
		{"dns", 1, 10, false, ""},
	} {
		got, err := raceStressOnly(root, c.module, c.runs, c.min)
		if got != c.want || (err == nil) != (c.err == "") || err != nil && !strings.Contains(err.Error(), c.err) {
			t.Errorf("%s runs %d minutes %d: %v, %v; want %v, %q", c.module, c.runs, c.min, got, err, c.want, c.err)
		}
	}
	if n := stressOnlyNote("websocket"); !strings.Contains(n, "std/websocket") || !strings.Contains(n, "#120") {
		t.Errorf("note: %s", n)
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

// TestRunSteps (#152): in parallel every step runs at once (each waits for
// all to start), the lines keep the steps' order, and an error is the
// first step's in that order.
func TestRunSteps(t *testing.T) {
	var started sync.WaitGroup
	started.Add(3)
	lines, err := runSteps([]string{"arm64", "llvm", "check"}, true, func(s string) (string, error) {
		started.Done()
		started.Wait()
		return "- " + s, nil
	})
	if err != nil || strings.Join(lines, ",") != "- arm64,- llvm,- check" {
		t.Fatalf("parallel: %q %v", lines, err)
	}
	var order []string
	lines, err = runSteps([]string{"a", "b"}, false, func(s string) (string, error) {
		order = append(order, s)
		return s, nil
	})
	if err != nil || strings.Join(order, "") != "ab" || strings.Join(lines, "") != "ab" {
		t.Fatalf("in turn: %q %q %v", order, lines, err)
	}
	_, err = runSteps([]string{"a", "b", "c"}, true, func(s string) (string, error) {
		if s != "a" {
			return "", errors.New("fail " + s)
		}
		return s, nil
	})
	if err == nil || err.Error() != "fail b" {
		t.Fatalf("error: %v", err)
	}
}

// TestHarnessesCoverEveryFuzz: every Test...Fuzz of internal/stdtest is a
// harness of some module, so stdproven fuzz runs it (TestMimeFuzz,
// TestPemFuzz and TestProtojsonFuzz were in none until 2026-11-17, and
// mime and protojson had no fuzz evidence to run). unregistered lists the
// exceptions with their reason.
func TestHarnessesCoverEveryFuzz(t *testing.T) {
	unregistered := map[string]string{}
	// the root when an earlier test moved there, else the package directory
	dir := "internal/stdtest"
	if _, err := os.Stat(dir); err != nil {
		dir = "../../internal/stdtest"
	}
	files, err := filepath.Glob(dir + "/*_test.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("internal/stdtest: %v (%d files)", err, len(files))
	}
	have := map[string]bool{}
	for _, hs := range harnesses {
		for _, h := range hs {
			have[h.test] = true
		}
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, l := range strings.Split(string(b), "\n") {
			name, ok := strings.CutPrefix(l, "func ")
			if !ok || !strings.HasPrefix(name, "Test") {
				continue
			}
			name, _, _ = strings.Cut(name, "(")
			if !strings.HasSuffix(name, "Fuzz") || have[name] || unregistered[name] != "" {
				continue
			}
			t.Errorf("%s (%s) is no module's harness: add it to harnesses in scripts/stdproven/main.go", name, filepath.Base(f))
		}
	}
}

// a harness with a fixed case count per seed (TestProtobufFuzz, 8,942)
// counts its own cases, not -batch, and stops once they reach -cases
func TestFuzzFixedCases(t *testing.T) {
	var clock time.Time
	f := fuzzRun{
		hs:    []harness{{"TestProtobufFuzz", "XO_PROTOFUZZ"}},
		cases: 20000, batch: 5000, backends: "arm64",
		limit: 8 * time.Hour,
		now:   func() time.Time { return clock },
		run: func(h harness, seed, batch int, backends string) ([]byte, error) {
			clock = clock.Add(time.Minute)
			return []byte("--- PASS: " + h.test + " (1s)\n"), nil
		},
	}
	st := fuzzResume("", "src1")
	if err := f.loop(&st); err != nil {
		t.Fatal(err)
	}
	ev := f.evidence(st, "protobuf", "abc", "src1", "h2", 8, "h2", time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC))
	for _, want := range []string{"result: pass\n", "TestProtobufFuzz 26826 cases per backend", "seeds: 2 to 4\n", "- TestProtobufFuzz seed 4: 8942 cases"} {
		if !strings.Contains(ev, want) {
			t.Errorf("evidence lacks %q:\n%s", want, ev)
		}
	}
}

// rssHelper is every stress program's rss_kib, which must return 0 under
// stdproven's --check=all run (stressCheckEnv).
func rssHelper(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	i := strings.Index(s, "\nfn rss_kib(os: Os) -> Int {\n")
	if i < 0 {
		t.Fatalf("%s: no fn rss_kib", path)
	}
	j := strings.Index(s[i:], "\n}\n")
	if j < 0 {
		t.Fatalf("%s: rss_kib has no end", path)
	}
	return s[i+1 : i+j+3]
}

// TestStressProgramsSkipRSSUnderCheck: one rss_kib in every stress
// program, reading the flag stressCheck sets (lead, 2026-11-17, #152:
// std/udp's --check=all run failed on memory alone, 0 mismatches).
func TestStressProgramsSkipRSSUnderCheck(t *testing.T) {
	progs, err := filepath.Glob(filepath.Join("..", "..", "std", "evidence", "*", "stress", "main.xo"))
	if err != nil || len(progs) == 0 {
		t.Fatalf("no stress programs: %v", err)
	}
	name, val, _ := strings.Cut(stressCheckEnv, "=")
	want := rssHelper(t, progs[0])
	if !strings.Contains(want, `os.env.get("`+name+`") ?? "") == "`+val+`" { return 0 }`) {
		t.Fatalf("%s: rss_kib does not return 0 under %s:\n%s", progs[0], stressCheckEnv, want)
	}
	for _, p := range progs[1:] {
		if got := rssHelper(t, p); got != want {
			t.Errorf("%s: rss_kib differs from %s's:\n%s", p, progs[0], got)
		}
	}
}

// TestStressCheckMemoryOnly: a program whose resident set grows with
// every result right passes stressCheck, and the leak report prints; the
// same binary run without the flag fails on memory, as std/udp's did.
// Linux only (rss_kib reads /proc), with the checkout's bin/xo.
func TestStressCheckMemoryOnly(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "arm64" {
		t.Skip("rss_kib reads /proc; --check=all builds arm64")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "bin", "xo")); err != nil {
		t.Skip("no bin/xo (go run ./scripts/xobuild)")
	}
	helper := rssHelper(t, filepath.Join(root, "std", "evidence", "udp", "stress", "main.xo"))
	dir := filepath.Join(t.TempDir(), "grow")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	prog := helper + `
fn main(os: Os) ! {
  let at_mark = rss_kib(os)
  var hold: List[Str] = []
  for i in 0..64 {
    hold.push("${i % 10}".repeat(1 << 20))
  }
  let last = rss_kib(os)
  let growth = if at_mark > 0 { Float(last - at_mark) * 100.0 / Float(at_mark) } else { 0.0 }
  let mem = if at_mark > 0 { "rss ${at_mark / 1024} MB at the mark, ${last / 1024} MB at the end (${growth.round()}%)" } else { "rss not sampled" }
  let mem_ok = at_mark == 0 || growth < 10.0 || last - at_mark < 4096
  os.stdio.println("grow stress: ${hold.len()} strings held, 0 mismatches, ${mem}, ${if mem_ok { "pass" } else { "FAIL (memory grew)" }}")
  if !mem_ok {
    return Err("memory grew ${growth}%")
  }
}
`
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	tmp := t.TempDir()
	line, note, err := stressCheck(dir, tmp, 1, 1)
	if err != nil {
		t.Fatalf("stressCheck: %v", err)
	}
	if !strings.Contains(line, "rss not sampled, pass; xo leakcheck: 0 live ") || note == "" {
		t.Errorf("line %q, note %q", line, note)
	}
	out, err := exec.Command(filepath.Join(tmp, "s-check"), "1", "1").CombinedOutput()
	if err == nil || !strings.Contains(string(out), "FAIL (memory grew)") {
		t.Errorf("without %s: %v, want a memory failure:\n%s", stressCheckEnv, err, out)
	}
}
