package arm64

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestWarmHelperOrder (task #476): a program edited and built again in
// the same cache is the file a build without the caches makes.
//
// testdata/warmhelper's lib.xo has rem (a Map[Int, Str] remove), other
// and fill (each a put). The first main calls all three, so rem's lowering
// makes xo.maph.oflog[Map[Int, Str]] (through xo.maph.slowrem) before
// other's makes xo.maph.slowput, which calls it, and fill finds both
// made. The second main calls only fill: a build without the cache makes
// slowput and then, from its body, oflog. fill's front end cache entry
// listed the two in the order the first program made them, so the second
// program's warm build made oflog first and its text had the two
// functions the other way round. The same entry made by a speculative
// front end worker gave one differing cold build in 150 of golden
// sql_conv (TestModuleCheckAuto on the gate hosts) and the warmseq
// failures of task #471. XO_FE_SPEC=1 here: every body is lowered at its
// turn, so the entry does not depend on timing.
func TestWarmHelperOrder(t *testing.T) {
	if !native() {
		t.Skip("the host is not arm64")
	}
	if testing.Short() {
		t.Skip("builds programs")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-warmhelper-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(work) })
	src := filepath.Join(work, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	put := func(from, to string) {
		t.Helper()
		b, err := os.ReadFile(filepath.Join("testdata", "warmhelper", from))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(src, to), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out := filepath.Join(work, "out", progExe)
	build := func(env ...string) ([]byte, string) {
		t.Helper()
		os.Remove(out)
		cmd := testtmp.Guard(exec.Command(xo, "build", "--backend=arm64", "--phase-times", "-o", out, "."))
		cmd.Dir = src
		cmd.Env = append(warmSeqEnv(), env...)
		log, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("xo build %v: %v\n%s", env, err, log)
		}
		b, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		return b, string(log)
	}
	warmEnv := []string{"XO_FE_SPEC=1", "XO_BACKEND_CACHE_DIR=" + filepath.Join(work, "cache")}
	coldEnv := []string{"XO_BACKEND_CACHE=off", "XO_FE_CACHE=off", "XO_CHK_CACHE=off", "XO_BIN_CACHE=off", "XO_SEPCOMP=0",
		"XO_BACKEND_CACHE_DIR=" + filepath.Join(work, "cold-cache")}
	put("lib.xo", "lib.xo")
	put("main_a.txt", "main.xo")
	build(warmEnv...)
	put("main_b.txt", "main.xo")
	warm, log := build(warmEnv...)
	// the test is of fill's entry replayed: its body lowered from source
	// (a miss) would pass whatever the entry holds
	if strings.Contains(log, "did not replay") {
		t.Fatalf("an entry did not replay:\n%s", log)
	}
	run, err := testtmp.Guard(exec.Command(out)).CombinedOutput()
	if err != nil || string(run) != "0 0\n" {
		t.Errorf("the warm build prints %q (%v), want \"0 0\\n\"", run, err)
	}
	cold, _ := build(coldEnv...)
	if !bytes.Equal(warm, cold) {
		t.Errorf("the second program's warm build differs from its build without the caches %s%s", warmDiff(warm, cold), keepWarmDiff("warmhelper", warm, cold))
	}
}
