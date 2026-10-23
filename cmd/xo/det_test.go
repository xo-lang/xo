package main

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// detProg's output is the order in which six tasks append to a shared
// list, which depends on how they interleave.
const detProg = `struct Log {
  order: List[Int]
}

fn main(os: Os) {
  let log = Mutex[Log].new(Log{order: []})
  let ch = Chan[Int].new(cap: 2)
  let n = Atomic[Int].new(0)
  scope s {
    for i in 0..6 {
      s.spawn(fn() {
        for k in 0..3 {
          log.with(fn(var l) { l.order.push(i * 10 + k) })
          ch.send(i)
          let _v = ch.recv()
          let _c = n.add(1)
        }
      })
    }
  }
  let o = log.with_read(fn(l) { l.order })
  os.stdio.println("${n.get()}: " + o.map(fn(x) { "${x}" }).join(" "))
}
`

// TestRunSeedNative: xo run --seed=N runs natively on the deterministic
// scheduler (decision 0350): one seed gives one interleaving every run,
// and the seeds give different ones of the same values.
func TestRunSeedNative(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds programs")
	}
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "main.xo"), []byte(detProg), 0o644); err != nil {
		t.Fatal(err)
	}
	runSeed := func(seed string) string {
		t.Helper()
		var out, errOut bytes.Buffer
		if code := run(t, []string{"run", "--seed=" + seed, src}, &out, &errOut); code != 0 {
			t.Fatalf("--seed=%s: exit %d stderr %s", seed, code, errOut.String())
		}
		return out.String()
	}
	first := runSeed("1")
	for i := 0; i < 2; i++ {
		if again := runSeed("1"); again != first {
			t.Fatalf("--seed=1 gave %q, then %q", first, again)
		}
	}
	values := func(s string) string {
		f := strings.Fields(strings.TrimPrefix(strings.TrimSpace(s), "18: "))
		sort.Strings(f)
		return strings.Join(f, " ")
	}
	want := values(first)
	if !strings.HasPrefix(first, "18: ") || len(strings.Fields(want)) != 18 {
		t.Fatalf("--seed=1: %q", first)
	}
	seen := map[string]bool{first: true}
	for _, seed := range []string{"2", "3", "4", "5", "6"} {
		got := runSeed(seed)
		if values(got) != want {
			t.Fatalf("--seed=%s: %q is not the same values as %q", seed, got, first)
		}
		seen[got] = true
	}
	if len(seen) < 2 {
		t.Fatalf("six seeds gave one interleaving: %q", first)
	}
}

// detManyProg puts 200 tasks on the deterministic scheduler's ready list
// at once, past its first capacity (64), so the list grows while it holds
// items (det.xo add: heap_large_resize takes the block's prefix; the data
// address faulted with SIGBUS in conc_stress, 2026-10-23).
const detManyProg = `fn main(os: Os) {
  let n = Atomic[Int].new(0)
  scope s {
    for i in 0..200 {
      s.spawn(fn() {
        let _c = n.add(i)
      })
    }
  }
  os.stdio.println("${n.get()}")
}
`

// TestRunSeedNativeManyTasks: more ready tasks than the ready list's first
// capacity, under --seed.
func TestRunSeedNativeManyTasks(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds programs")
	}
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "main.xo"), []byte(detManyProg), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if code := run(t, []string{"run", "--seed=7", src}, &out, &errOut); code != 0 {
		t.Fatalf("exit %d stderr %s", code, errOut.String())
	}
	if got := strings.TrimSpace(out.String()); got != "19900" {
		t.Fatalf("got %q, want 19900", got)
	}
}

// TestTestExploreNative: `xo test --explore=N` runs natively on the
// deterministic scheduler (decision 0350 stage 2): it finds the planted
// lost update of internal/gen's test_explore golden under some seed, the
// printed `--seed=S --run=...` reproduces that failure every time, and
// the race free test passes under every seed.
func TestTestExploreNative(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds programs")
	}
	dir := filepath.Join("..", "..", "internal", "gen", "testdata", "golden", "test_explore")
	xoTest := func(args ...string) (int, string) {
		t.Helper()
		var out, errOut bytes.Buffer
		code := run(t, append([]string{"test"}, append(args, dir)...), &out, &errOut)
		return code, out.String() + errOut.String()
	}
	code, out := xoTest("--explore=40", "--run=lost update")
	if code != 1 {
		t.Fatalf("--explore=40: exit %d, want 1:\n%s", code, out)
	}
	m := regexp.MustCompile("--- FAIL: lost update \\(main_test.xo:1:1, seed (\\d+)\\)\n    explored seeds 1 to (\\d+): seed (\\d+) fails; reproduce with `xo test --seed=(\\d+) --run='lost update'`").FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("--explore=40 found no failing seed of `lost update`:\n%s", out)
	}
	seed := m[1]
	if m[2] != seed || m[3] != seed || m[4] != seed {
		t.Fatalf("the seeds disagree: %q", m[0])
	}
	// a plain run first: the run cache must not hand the seeded runs its
	// thread pool binary (--seed left the cache key, 2026-10-23)
	xoTest("--run=lost update")
	first := ""
	for i := 0; i < 5; i++ {
		code, got := xoTest("--seed="+seed, "--run=lost update")
		if code != 1 || !strings.Contains(got, "--- FAIL: lost update (main_test.xo:1:1, seed "+seed+")") {
			t.Fatalf("--seed=%s: exit %d, the failure does not reproduce:\n%s", seed, code, got)
		}
		if i == 0 {
			first = got
		} else if got != first {
			t.Fatalf("--seed=%s printed\n%s\nthen\n%s", seed, first, got)
		}
	}
	if code, got := xoTest("--explore=40", "--run=no race"); code != 0 {
		t.Fatalf("the race free test failed under --explore=40: exit %d\n%s", code, got)
	}
}

// TestTestSeedFakeClock: a test that sleeps on the test world's virtual
// clock finishes under --seed (decision 0350 stage 2): the fibers the
// clock's kick wakes go on the deterministic ready list, which pool.xo
// any_work must count, or the idle root parks for good (every fake clock
// test hit the real time limit, 2026-10-23).
func TestTestSeedFakeClock(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds programs")
	}
	src := t.TempDir()
	for name, body := range map[string]string{
		"main.xo":      "fn main(os: Os) {\n}\n",
		"main_test.xo": "test \"sleep on the fake clock\" {\n  os.clock.sleep(5s)\n  expect(1) == 1\n}\n",
	} {
		if err := os.WriteFile(filepath.Join(src, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var out, errOut bytes.Buffer
	if code := run(t, []string{"test", "--seed=3", "--timeout=30s", src}, &out, &errOut); code != 0 {
		t.Fatalf("exit %d\n%s%s", code, out.String(), errOut.String())
	}
}

// TestExploreFirstSeedCommand: a test that fails on --explore's first
// seed prints the exact command that reproduces it, as a later seed's
// failure does, on the Go backend and natively, and that command fails
// the test again (field report 0.26.0: the Go backend said only "seed 1",
// and `xo test --seed=1` without --backend=go built natively and passed).
func TestExploreFirstSeedCommand(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds programs")
	}
	src := t.TempDir()
	for name, body := range map[string]string{
		"main.xo":      "fn main(os: Os) {\n}\n",
		"main_test.xo": "test \"always fails\" {\n  let n = Atomic[Int].new(0)\n  scope s {\n    s.spawn(fn() { let _ = n.add(1) })\n  }\n  expect(n.get()) == 2\n}\n",
	} {
		if err := os.WriteFile(filepath.Join(src, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	xoTestOut := func(args ...string) (int, string) {
		t.Helper()
		var out, errOut bytes.Buffer
		code := run(t, append(append([]string{"test"}, args...), src), &out, &errOut)
		return code, out.String() + errOut.String()
	}
	cmdRE := regexp.MustCompile("reproduce with `xo test ([^`]*)`")
	for _, backend := range []string{"go", ""} {
		args := []string{"--explore=3"}
		if backend != "" {
			args = append(args, "--backend="+backend)
		}
		code, out := xoTestOut(args...)
		if code != 1 {
			t.Fatalf("%v: exit %d, want 1\n%s", args, code, out)
		}
		m := cmdRE.FindStringSubmatch(out)
		if m == nil || !strings.Contains(m[1], "--seed=1") {
			t.Fatalf("%v: no reproduce command for seed 1:\n%s", args, out)
		}
		// the command's words as a shell splits them: flags, and the
		// quoted test name of --run
		var again []string
		for _, w := range regexp.MustCompile(`--[a-z]+(=('[^']*'|\S+))?`).FindAllString(m[1], -1) {
			again = append(again, strings.ReplaceAll(w, "'", ""))
		}
		if code, out := xoTestOut(again...); code != 1 || !strings.Contains(out, "--- FAIL: always fails") {
			t.Fatalf("%v: the printed command %q does not fail the test again (exit %d):\n%s", args, m[1], code, out)
		}
	}
}
