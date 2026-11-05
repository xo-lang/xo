package main

import (
	"bytes"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

const heapStatsProg = `fn main(os: Os) {
  // 100000 Strs of a 32 byte class built and dropped single threaded:
  // 100000 blocks (40 bytes each with the header) on the free list
  var xs: List[Str] = []
  for i in 0..100000 {
    xs.push("key${i}")
  }
  os.stdio.println("${xs.len()}")
  xs = []
  os.stdio.println("${xs.len()}")
}
`

var heapStatsLine = regexp.MustCompile(`on free lists (\d+) MB`)

// TestHeapStatsFreeLists: XO_HEAPSTATS=1 reports the memory on the free
// lists of a program that built and dropped 100000 Strs single threaded
// as the 4 MB they are (the sum read the records' per class counts, which
// that mode leaves alone: 0 MB here, and 17592186044415 MB when pops
// outnumbered the counted frees).
func TestHeapStatsFreeLists(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds and runs a native program")
	}
	setenv(t, "XO_BUILD_DIR", filepath.Join(t.TempDir(), "build"))
	dir := writeFiles(t, map[string]string{"main.xo": heapStatsProg})
	for _, be := range nativeBackends(t) {
		res, err := xotool.Build([]string{dir}, buildOpts(t, xotool.Options{Backend: be, Out: filepath.Join(t.TempDir(), "prog")}))
		if err != nil || res.Failed {
			t.Fatalf("build on %s: %v %s", be, err, res.Output)
		}
		cmd := command(t, res.Binaries[xotool.Host()])
		cmd.Env = append(cmd.Env, "XO_HEAPSTATS=1")
		var errb bytes.Buffer
		cmd.Stderr = &errb
		if err := cmd.Run(); err != nil {
			t.Fatalf("run on %s: %v\n%s", be, err, errb.String())
		}
		m := heapStatsLine.FindStringSubmatch(errb.String())
		if m == nil {
			t.Fatalf("%s: no free list figure:\n%s", be, errb.String())
		}
		mb, _ := strconv.Atoi(m[1])
		if mb < 3 || mb > 8 {
			t.Errorf("%s: %d MB on free lists, want about 4 (100000 blocks of 40 bytes):\n%s", be, mb, errb.String())
		}
	}
}

const heapWasteProg = `fn main(os: Os) {
  // 2000 Bytes of 3000 bytes each: blocks of the 4088 byte class (decision
  // 0506's measurement, XO_HEAPSTATS=waste)
  var xs: List[Bytes] = []
  for i in 0..2000 {
    xs.push(b"x".repeat(3000 + i % 2))
  }
  os.stdio.println("${xs.len()}")
}
`

var heapWasteLine = regexp.MustCompile(`waste class 66 allocs (\d+) asked (\d+) handed (\d+) per mille (\d+)`)

// TestHeapStatsWaste: XO_HEAPSTATS=waste reports, per size class above
// 1 KiB, what allocations asked for against what their class handed out.
func TestHeapStatsWaste(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds and runs a native program")
	}
	setenv(t, "XO_BUILD_DIR", filepath.Join(t.TempDir(), "build"))
	dir := writeFiles(t, map[string]string{"main.xo": heapWasteProg})
	for _, be := range nativeBackends(t) {
		res, err := xotool.Build([]string{dir}, buildOpts(t, xotool.Options{Backend: be, Out: filepath.Join(t.TempDir(), "prog")}))
		if err != nil || res.Failed {
			t.Fatalf("build on %s: %v %s", be, err, res.Output)
		}
		cmd := command(t, res.Binaries[xotool.Host()])
		cmd.Env = append(cmd.Env, "XO_HEAPSTATS=waste")
		var errb bytes.Buffer
		cmd.Stderr = &errb
		if err := cmd.Run(); err != nil {
			t.Fatalf("run on %s: %v\n%s", be, err, errb.String())
		}
		m := heapWasteLine.FindStringSubmatch(errb.String())
		if m == nil {
			t.Fatalf("%s: no class 66 waste line:\n%s", be, errb.String())
		}
		allocs, _ := strconv.Atoi(m[1])
		asked, _ := strconv.Atoi(m[2])
		handed, _ := strconv.Atoi(m[3])
		if allocs < 2000 || handed != allocs*4096 || asked < allocs*3000 || asked > allocs*3100 {
			t.Errorf("%s: class 66 allocs %d asked %d handed %d, want at least 2000 blocks of 4096 bytes for about 3000 asked each:\n%s", be, allocs, asked, handed, errb.String())
		}
	}
}
