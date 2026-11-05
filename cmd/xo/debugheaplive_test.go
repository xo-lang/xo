package main

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// heap_live from /debug/sched rises by the blocks a program holds and falls
// back when it lets them go (decision 0520's soak reads it while the
// program runs).
const heapLiveProg = `use std/debug
use std/http

fn live(r: http.Router) -> Int {
  let text = r.call(http.new_request("GET", "/sched")).text() ?? ""
  for line in text.split("\n") {
    if line.starts_with("heap_live ") {
      return Int.parse(line.trim_prefix("heap_live ")) ?? -2
    }
  }
  -1
}

fn hold(r: http.Router, n: Int) -> Int {
  var keep: List[Bytes] = List.with_capacity(n)
  for i in 0..n {
    keep.push(b"x".repeat(32 + i % 7))
  }
  let during = live(r)
  var t = 0
  for b in keep {
    t += b.len()
  }
  if t == 0 { -3 } else { during }
}

fn main(os: Os) {
  let r = debug.router(os.clock, fn(_) { true })
  let before = live(r)
  let during = hold(r, 100000)
  let after = live(r)
  os.stdio.println("live ${before} ${during} ${after}")
}
`

func TestDebugSchedHeapLive(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds and runs a program")
	}
	dir := writeFiles(t, map[string]string{
		"app/xo.mod":  "module app\n",
		"app/main.xo": heapLiveProg,
	})
	app := filepath.Join(dir, "app")
	for _, be := range nativeBackends(t) {
		bin := filepath.Join(t.TempDir(), "app")
		if code, out := xoOut(t, "build", "--backend="+be, "-o", bin, app); code != 0 {
			t.Fatalf("%s: build: exit %d\n%s", be, code, out)
		}
		cmd := command(t, bin)
		cmd.Env = append(cmd.Environ(), "XO_RTSTATS=1")
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("%s: run: %v\n%s", be, err, out)
		}
		f := strings.Fields(strings.TrimSpace(string(out)))
		if len(f) != 4 || f[0] != "live" {
			t.Fatalf("%s: output %q", be, out)
		}
		before, _ := strconv.Atoi(f[1])
		during, _ := strconv.Atoi(f[2])
		after, _ := strconv.Atoi(f[3])
		t.Logf("%s: heap_live %d before, %d while holding 100,000 blocks, %d after", be, before, during, after)
		if before < 0 || during-before < 100000 || after-before > 1000 {
			t.Errorf("%s: heap_live %d, %d, %d: want a rise of at least 100,000 while held and back within 1,000 after", be, before, during, after)
		}
		// without counting there is no line
		out, err = command(t, bin).Output()
		if err != nil {
			t.Fatalf("%s: run without counting: %v\n%s", be, err, out)
		}
		if !strings.HasPrefix(strings.TrimSpace(string(out)), "live -1 -1 -1") {
			t.Errorf("%s: without XO_RTSTATS got %q, want no heap_live line (-1)", be, out)
		}
	}
}
