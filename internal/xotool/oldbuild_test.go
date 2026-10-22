package xotool

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// oldBuildCommit is a main commit from before decision 0318 stage 4
// (333dd3311) and before the 0.24.0 seed's commit (e2a31201), whose std
// has no xo_x_preempt: the newest seed's output calls it.
const oldBuildCommit = "f3da8689066f38e55d381855a74c3fece22bfe86"

// TestBuildOldCommit (XO_TEST_OLD_BUILD=1, several minutes): Ensure
// builds a commit older than the newest seed, as a bisect step does,
// into an xo that runs a program. On 2026-10-22 seed 0.24.0 built
// d33656fda into an xo that dyld stopped at `Symbol not found:
// _xo_x_preempt` (gotchas/xobuild-seed-newer-than-commit.md). The
// checkout cache and the back-end cache are private (an old xo's prune
// must not touch the shared one); the unpacked seeds are shared.
func TestBuildOldCommit(t *testing.T) {
	if os.Getenv("XO_TEST_OLD_BUILD") != "1" {
		t.Skip("XO_TEST_OLD_BUILD=1 builds an old commit's xo (several minutes)")
	}
	root := repoRoot(t)
	if !knownCommit(root, oldBuildCommit) {
		t.Skipf("the repository has no commit %s", oldBuildCommit)
	}
	base, err := cacheBase()
	if err != nil {
		t.Fatal(err)
	}
	cache := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base, "seeds"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(base, "seeds"), filepath.Join(cache, "seeds")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XO_CHECKOUT_CACHE", cache)
	t.Setenv("XO_BACKEND_CACHE_DIR", t.TempDir())
	t.Setenv("XO_SEED", "")
	t.Setenv("XO_CLI", "")
	dir := filepath.Join(t.TempDir(), "xo")
	for _, args := range [][]string{
		{"clone", "-q", "--shared", "--no-checkout", root, dir},
		{"-C", dir, "checkout", "-q", "--detach", oldBuildCommit},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	var log bytes.Buffer
	bin, err := Ensure(dir, &log)
	t.Logf("%s", log.String())
	if err != nil {
		t.Fatal(err)
	}
	prog := filepath.Join(t.TempDir(), "main.xo")
	if err := os.WriteFile(prog, []byte("fn main(os: Os) {\n  os.stdio.println(\"old xo runs\")\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runIn(filepath.Dir(prog), nil, bin, "run", filepath.Dir(prog))
	if err != nil || !strings.Contains(out, "old xo runs") {
		t.Fatalf("%s run: %v\n%s", bin, err, out)
	}
}
