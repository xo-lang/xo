package arm64

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestWarmBuildWritesLittle: a warm build of an unchanged program on
// linux writes almost nothing to xo's caches. Its part objects reach Xo's
// ELF linker from memory (link.xo Ctx mem_objs), not as a copy of the
// cached bytes in the build's directory, and std's sums memo is stored
// only when its text changes; before, every warm hello wrote 3.6 MB
// (17 part objects and the memo, 2026-11-06). The program still runs.
func TestWarmBuildWritesLittle(t *testing.T) {
	t.Parallel()
	if !native() || runtime.GOOS != "linux" {
		t.Skip("linux/arm64 only (darwin's debug map needs the object files)")
	}
	if testing.Short() {
		t.Skip("builds a program five times")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-warmw-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	prog := filepath.Join(work, progExe)
	if err := os.MkdirAll(prog, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(prog, "main.xo"), []byte("fn main(os: Os) {\n  let ws = \"a b c\".split(\" \")\n  os.stdio.println(\"${ws.len()} ${ws[2].to_upper()}\")\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(work, "bin")
	build := func() string {
		t.Helper()
		cmd := testtmp.Guard(exec.Command(xo, "build", "--backend=arm64", "-o", bin, "."))
		cmd.Dir = prog
		// whole program, so the parts are this build's own
		cmd.Env = append(os.Environ(), "XO_BACKEND_CACHE=on", "XO_BACKEND_CACHE_DIR="+filepath.Join(work, "cache"), "XO_BIN_CACHE=off", "XO_SEPCOMP=0", "XO_CACHE_STATS=1")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("xo build: %v\n%s", err, out)
		}
		return string(out)
	}
	// the caches settle over four builds: the first stores the entries,
	// the second and third form the target's packs, the fourth the front
	// end's list of keys found nowhere (decision 0351); from the fifth a
	// build writes only its build information object (1.3 KB on
	// linux/arm64, 2026-11-06)
	for i := 0; i < 4; i++ {
		build()
	}
	log := build()
	written := 0
	for _, m := range regexp.MustCompile(`(?m)^cache stats: (entry|pack|shared) write: \d+ calls, (\d+) bytes$`).FindAllStringSubmatch(log, -1) {
		n, _ := strconv.Atoi(m[2])
		written += n
	}
	if written > 64<<10 {
		t.Errorf("a warm build of an unchanged program wrote %d bytes to the caches, want at most 64 KiB:\n%s", written, log)
	}
	out, err := testtmp.Guard(exec.Command(bin)).CombinedOutput()
	if err != nil || string(out) != "3 C\n" {
		t.Fatalf("%s: %v %q, want \"3 C\\n\"", bin, err, out)
	}
}
