package main

import (
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// memTrimProg frees 10,240 blocks of the 16 KiB class (12,000 bytes
// each, 160 MB of blocks) in tasks, so on worker threads' heap records and
// the central lists' magazines, then trims from main and prints its
// resident set before and after, in MB, from /proc/self/statm.
const memTrimProg = `use std/debug

fn rss_mb(os: Os) -> Int {
  let t = os.fs.read_text("/proc/self/statm").ok() ?? ""
  let parts = t.split(" ")
  if parts.len() < 2 {
    return -1
  }
  (Int.parse(parts[1]) ?? 0) * 4096 >> 20
}

fn main(os: Os) {
  // every task holds its blocks until all 8 have allocated (a barrier of
  // channels, which park the task, not its thread), so the blocks are all
  // live at once however many threads run the tasks: on a loaded machine
  // the tasks ran partly one after another and reused each other's
  // blocks, and the resident set before the trim fell toward the test's
  // 60 MB bound (67 MB in a full cmd/xo run, 2026-11-05)
  let ready: Chan[Int] = Chan.new(8)
  let go: Chan[Int] = Chan.new(8)
  scope s {
    s.spawn(fn() {
      for _ in 0..8 {
        let _r = ready.recv()
      }
      for _ in 0..8 {
        go.send(1)
      }
    })
    for k in 0..8 {
      s.spawn(fn() {
        var keep: List[Bytes] = []
        for _ in 0..1280 {
          keep.push(b"t".repeat(12000))
        }
        ready.send(k)
        let _g = go.recv()
        var n = 0
        for b in keep {
          n += b.len() + k
        }
        n
      })
    }
  }
  let before = rss_mb(os)
  debug.mem_trim()
  let after = rss_mb(os)
  os.stdio.println("rss ${before} ${after}")
}
`

// TestMemTrimOtherThreads checks decision 0505's addendum: debug.mem_trim
// gives back the free size class blocks of every worker's heap record and
// of the central lists, not only the calling thread's. Before it, 16 KiB
// blocks freed by tasks stayed resident through the trim on Linux. Linux
// only: the resident set is read from /proc, and darwin compresses the
// freed pages, which hides the difference.
func TestMemTrimOtherThreads(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds and runs a program")
	}
	if runtime.GOOS != "linux" {
		t.Skip("reads the resident set from /proc/self/statm")
	}
	dir := writeFiles(t, map[string]string{
		"app/xo.mod":  "module app\n",
		"app/main.xo": memTrimProg,
	})
	app := filepath.Join(dir, "app")
	for _, be := range nativeBackends(t) {
		bin := filepath.Join(t.TempDir(), "app")
		if code, out := xoOut(t, "build", "--backend="+be, "-o", bin, app); code != 0 {
			t.Fatalf("%s: build: exit %d\n%s", be, code, out)
		}
		out, err := command(t, bin).CombinedOutput()
		if err != nil {
			t.Fatalf("%s: run: %v\n%s", be, err, out)
		}
		f := strings.Fields(strings.TrimSpace(string(out)))
		if len(f) != 3 || f[0] != "rss" {
			t.Fatalf("%s: output %q", be, out)
		}
		before, _ := strconv.Atoi(f[1])
		after, _ := strconv.Atoi(f[2])
		t.Logf("%s: resident %d MB before the trim, %d MB after", be, before, after)
		// about 120 MB of blocks were freed (91 to 162 MB resident before
		// the trim, by build and date); the bound is on what stays (Go's
		// FreeOSMemory leaves 6 to 8 MB of the probe's; without 0505's
		// trim nothing came back, 148 MB before and after)
		if before < 60 || after > 40 {
			t.Errorf("%s: %d MB resident before the trim, %d after (want at least 60 before, at most 40 after)", be, before, after)
		}
	}
}
