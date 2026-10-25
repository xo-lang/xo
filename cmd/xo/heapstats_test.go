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
  // a single threaded loop of allocations and frees: the pops of blocks
  // that were never counted when freed once drove the per class counts
  // negative
  var keep: List[Str] = []
  for i in 0..20000 {
    let s = "key${i}"
    if i % 3 == 0 {
      keep.push(s)
    }
  }
  os.stdio.println("${keep.len()}")
}
`

var heapStatsLine = regexp.MustCompile(`on free lists (\d+) MB`)

// TestHeapStatsFreeLists: XO_HEAPSTATS=1 reports the memory on the free
// lists of a program that allocated and freed single threaded as a small
// figure (the sum read the records' per class counts, which that mode
// leaves alone, and printed 17592186044415 MB).
func TestHeapStatsFreeLists(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs a native program")
	}
	setenv(t, "XO_BUILD_DIR", filepath.Join(t.TempDir(), "build"))
	dir := writeFiles(t, map[string]string{"main.xo": heapStatsProg})
	for _, be := range nativeBackends(t) {
		res, err := xotool.Build([]string{dir}, xotool.Options{Backend: be, Out: filepath.Join(t.TempDir(), "prog")})
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
		if mb > 64 {
			t.Errorf("%s: %d MB on free lists of a program that allocated about 2 MB:\n%s", be, mb, errb.String())
		}
	}
}
