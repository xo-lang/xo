package arm64

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestStdRecordJsonVia: a std type's own JSON form (`derive Json(via:,
// to:, from:)`, decision 0478) reaches an importer built from std's
// record (decision 0388 stage 1a-ii), the default of a separately
// compiled build. testdata/stdrecord_via decodes and encodes a struct
// holding every std type that has one (time.Date, uuid.Uuid,
// sql.Numeric, big.BigInt, big.BigFloat, big.BigRat) and three bad
// values their `from` functions refuse. The cache holds std's variants
// (xo dev stdcache), so the first build writes the record and the second
// loads it: its program prints what a whole-program build's does.
// Before (task #566, 2026-11-20), the record was written before
// check_std_json_via set the forms, and a module made from the record
// has no tree for it to read later: the second build derived plain
// codecs ("err at $.u: expected object", and a Date's range checks were
// skipped: golden time_hostile_date in the linux/amd64 full runs of
// tiers 79 and 918, after any program had written the record).
func TestStdRecordJsonVia(t *testing.T) {
	if !native() {
		t.Skip("the host is not arm64")
	}
	if testing.Short() {
		t.Skip("builds std's variants and programs")
	}
	t.Parallel()
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-stdrecvia-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(work) })
	// xo's defaults (no cache, separate compilation or record setting
	// of the caller's), on a cache of its own
	var env []string
	for _, kv := range warmSeqEnv() {
		if k, _, _ := strings.Cut(kv, "="); strings.HasPrefix(k, "XO_SEPCOMP") || k == "XO_STD_EXPORT" || k == "XO_WHOLE_BUILD" {
			continue
		}
		env = append(env, kv)
	}
	env = append(env, "XO_BACKEND_CACHE=on", "XO_BACKEND_CACHE_DIR="+filepath.Join(work, "cache"), "XO_BIN_CACHE=off", "XO_CACHE_STATS=1")
	for _, args := range [][]string{{"dev", "stdcache"}, {"dev", "stdcache", "--conc=1"}} {
		cmd := testtmp.Guard(exec.Command(xo, args...))
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("xo %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	dir := filepath.Join("testdata", "stdrecord_via")
	build := func(name string, env []string) (string, string) {
		bin := filepath.Join(work, name, progExe)
		cmd := testtmp.Guard(exec.Command(xo, "build", "-o", bin, "."))
		cmd.Dir = dir
		cmd.Env = env
		log, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s: xo build: %v\n%s", name, err, log)
		}
		out, err := testtmp.Guard(exec.Command(bin)).CombinedOutput()
		if err != nil {
			t.Fatalf("%s: run: %v\n%s", name, err, out)
		}
		return string(out), string(log)
	}
	want, _ := build("whole", append(append([]string{}, env...), "XO_SEPCOMP=0", "XO_BACKEND_CACHE_DIR="+filepath.Join(work, "cache-whole")))
	if !strings.HasPrefix(want, `ok {"d":"2026-11-14","u":"123e4567-e89b-12d3-a456-426614174000","n":12.50,"i":123456789012345678901234567890,"f":"1.5","r":"3/4"}`) || strings.Contains(want, "accepted") || strings.Count(want, "\nerr at $.") != 3 {
		t.Fatalf("the whole-program build prints\n%s", want)
	}
	first, log1 := build("first", env)
	if !strings.Contains(log1, "cache stdx no entry") {
		t.Fatalf("the first build did not write std's record (no separate compilation here?):\n%s", log1)
	}
	second, log2 := build("second", env)
	if !strings.Contains(log2, "cache stdx record loaded") || strings.Contains(log2, "cache stdx declared") {
		t.Fatalf("the second build did not use std's record:\n%s", log2)
	}
	if first != want {
		t.Errorf("the build that wrote std's record prints\n%s\nwant (the whole-program build)\n%s", first, want)
	}
	if second != want {
		t.Errorf("the build from std's record prints\n%s\nwant (the whole-program build)\n%s", second, want)
	}
}
