package xotool

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
)

// TestTierSepcompWarm: with std's variants cached, a build without
// XO_SEPCOMP compiles separately (decision 0388's default), and the
// environment XO_TEST_SEPCOMP=0 gives the tier (tierSepcomp) builds the
// whole program, which skipping the variants' prebuild alone did not.
func TestTierSepcompWarm(t *testing.T) {
	if testing.Short() {
		t.Skip("builds std's variants")
	}
	if (runtime.GOOS != "darwin" && runtime.GOOS != "linux") || (runtime.GOARCH != "arm64" && runtime.GOARCH != "amd64") {
		t.Skip("no separate compilation by default on this host")
	}
	cli, err := Command()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-tiersep-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	prog := filepath.Join(work, "prog")
	if err := os.MkdirAll(prog, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(prog, "main.xo"), []byte("fn main(os: Os) {\n  os.stdio.println(\"hi\")\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// the test's own environment without XO_SEPCOMP (the tier sets it)
	var base []string
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "XO_SEPCOMP=") {
			base = append(base, e)
		}
	}
	base = append(base, "XO_BACKEND_CACHE=on", "XO_BACKEND_CACHE_DIR="+filepath.Join(work, "cache"), "XO_SEPCOMP_BG=0", "XO_BIN_CACHE=off")
	run := func(args []string, env []string) string {
		t.Helper()
		cmd := testtmp.Guard(exec.Command(cli[0], append(append([]string{}, cli[1:]...), args...)...))
		cmd.Dir = prog
		cmd.Env = append(append([]string{}, base...), env...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("xo %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	for _, c := range []string{"0", "1"} {
		run([]string{"dev", "stdcache", "--conc=" + c}, nil)
	}
	build := []string{"build", "--phase-times", "-o", filepath.Join(work, "bin"), "."}
	if log := run(build, nil); !strings.Contains(log, "cache std ") {
		t.Fatalf("with std's variants cached the default build was whole:\n%s", log)
	}
	k, v, ok := tierSepcomp(func(n string) string {
		if n == "XO_TEST_SEPCOMP" {
			return "0"
		}
		return ""
	})
	if !ok {
		t.Fatal("tierSepcomp set nothing for XO_TEST_SEPCOMP=0")
	}
	if log := run(build, []string{k + "=" + v}); strings.Contains(log, "cache std ") {
		t.Fatalf("XO_TEST_SEPCOMP=0 with std's variants cached built separately:\n%s", log)
	}
}
