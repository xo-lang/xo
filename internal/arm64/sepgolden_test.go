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

// sepcompGoldens are the goldens TestSepcompGoldens builds separately
// compiled: scripts/docker-sepcomp-sample.txt's std-heavy set, and
// byref_struct, whose fault exits store a struct's zero from a shared
// image that once named a string literal of another part (2026-11-02:
// a store-mode literal is a symbol local to its part's object, so the
// image's part could not link it).
var sepcompGoldens = []string{"byref_struct"}

// TestSepcompGoldens builds goldens the way a build outside the tests
// does once std's variants are cached (decision 0388 stage 3 step (d):
// the program against std's store), and requires their want.txt. The
// other golden tests build whole programs: xotool turns the background
// variant build off and their caches hold no variant, so a fault only
// separate compilation has passed every golden test.
func TestSepcompGoldens(t *testing.T) {
	skipWholeTier(t)
	if !native() {
		t.Skip("the host is not arm64")
	}
	if testing.Short() {
		t.Skip("builds std's variants and programs")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-sepgold-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	cache := []string{"XO_BACKEND_CACHE=on", "XO_BACKEND_CACHE_DIR=" + filepath.Join(work, "cache")}
	for _, conc := range []string{"0", "1"} {
		cmd := testtmp.Guard(exec.Command(xo, "dev", "stdcache", "--conc="+conc))
		cmd.Env = append(os.Environ(), cache...)
		if out, err := stdRun(cmd.CombinedOutput); err != nil {
			t.Fatalf("xo dev stdcache --conc=%s: %v\n%s", conc, err, out)
		}
	}
	// the build takes std's variants (gotchas/sepcomp-default-check-
	// needs-std-variants.md: a whole-program build passes trivially)
	probe := testtmp.Guard(exec.Command(xo, "build", "--backend=arm64", "--phase-times", "-o", filepath.Join(work, "probe"), "."))
	probe.Dir = filepath.Join("..", "llvm", "testdata", "golden", "lists")
	probe.Env = append(os.Environ(), cache...)
	if out, err := probe.CombinedOutput(); err != nil || !strings.Contains(string(out), "conc=false reused") {
		t.Fatalf("a build did not take std's variants (no `cache std KEY conc=false reused`): %v\n%s", err, out)
	}
	names := append([]string{}, sepcompGoldens...)
	sample, err := os.ReadFile(filepath.Join("..", "..", "scripts", "docker-sepcomp-sample.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range strings.Split(string(sample), "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "#") {
			names = append(names, l)
		}
	}
	for _, name := range names {
		dir := filepath.Join("..", "llvm", "testdata", "golden", name)
		t.Run(name, func(t *testing.T) {
			want, err := os.ReadFile(filepath.Join(dir, "want.txt"))
			if err != nil {
				t.Fatal(err)
			}
			got, err := buildRunWith(t, dir, xotool.Options{Backend: xotool.BackendArm64, Env: cache})
			if err != nil {
				t.Fatal(err)
			}
			if string(want) != got.String() {
				t.Errorf("output differs from want.txt separately compiled\n--- want\n%s\n--- got\n%s", want, got)
			}
		})
	}
}
