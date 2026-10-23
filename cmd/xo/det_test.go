package main

import (
	"bytes"
	"os"
	"path/filepath"
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
