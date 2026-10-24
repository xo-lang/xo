package stdsync

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/stdindex"
)

// tree makes a repository root with prelude, the stamp the habit goldens
// would have been made from, and a spec whose index comes from indexFrom
// (the prelude itself for a current one).
func tree(t *testing.T, prelude, indexFrom string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "std"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "spec"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, PreludePath), []byte(prelude), 0o644); err != nil {
		t.Fatal(err)
	}
	spec := stdindex.Splice("# std\n\nprose\n", stdindex.Index(indexFrom))
	if err := os.WriteFile(filepath.Join(root, SpecPath), []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(stdindex.Index(prelude)))
	stampPath := filepath.Join(root, HabitsStamp)
	if err := os.MkdirAll(filepath.Dir(stampPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stampPath, []byte(hex.EncodeToString(sum[:])+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestRunFixesStaleIndex is the whole point: a stale generated file in,
// the fixed file out, and check reports it without writing.
func TestRunFixesStaleIndex(t *testing.T) {
	const prelude = "fn (s: Str) len() -> Int\nfn fault(msg: Str) -> Never\n"
	root := tree(t, prelude, "fn (s: Str) old() -> Int\n")
	specPath := filepath.Join(root, SpecPath)
	before := read(t, specPath)

	stale, err := Run(Options{Root: root}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(stale) != 1 || stale[0].Artifact != SpecPath || stale[0].Detail != "is stale" {
		t.Fatalf("check reported %v, want spec/std.md is stale", stale)
	}
	if got := read(t, specPath); got != before {
		t.Fatalf("check wrote spec/std.md:\n%s", got)
	}

	if _, err := Run(Options{Root: root}, false); err != nil {
		t.Fatal(err)
	}
	if got, want := stdindex.Current(read(t, specPath)), stdindex.Index(prelude); got != want {
		t.Fatalf("after the sync the index is not the prelude's:\n%s", got)
	}
	stale, err = Run(Options{Root: root}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(stale) != 0 {
		t.Fatalf("after the sync, check reports %v", stale)
	}
}

// TestRunHabitsStamp: the habit goldens are regenerated only when the
// prelude's declarations changed, and the stamp records what they were made
// from.
func TestRunHabitsStamp(t *testing.T) {
	const prelude = "fn (s: Str) len() -> Int\n"
	root := tree(t, prelude, prelude)
	if err := os.Remove(filepath.Join(root, HabitsStamp)); err != nil {
		t.Fatal(err)
	}
	runs := 0
	o := func() Options {
		return Options{Root: root, Habits: func(string) error { runs++; return nil }}
	}

	stale, err := Run(o(), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(stale) != 1 || stale[0].Artifact != HabitsStamp || !strings.Contains(stale[0].Detail, "missing") {
		t.Fatalf("check reported %v, want the missing stamp", stale)
	}
	if runs != 0 {
		t.Fatalf("check regenerated the habit goldens")
	}

	if _, err := Run(o(), false); err != nil {
		t.Fatal(err)
	}
	if runs != 1 {
		t.Fatalf("the write run called Habits %d times, want 1", runs)
	}
	if _, err := Run(o(), true); err != nil {
		t.Fatal(err)
	}
	if runs != 1 {
		t.Fatalf("a current stamp regenerated the habit goldens (%d calls)", runs)
	}

	// a prelude declaration changes: stale again, and the next write run
	// regenerates them behind the same hash
	if err := os.WriteFile(filepath.Join(root, PreludePath), []byte(prelude+"fn (s: Str) find(sub: Str) -> Int?\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stale, err = Run(o(), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(stale) != 2 {
		t.Fatalf("a prelude change reported %v, want the index and the stamp", stale)
	}
	if _, err := Run(o(), false); err != nil {
		t.Fatal(err)
	}
	if runs != 2 {
		t.Fatalf("a changed prelude called Habits %d times, want 2", runs)
	}
	if _, err := Run(o(), true); err != nil {
		t.Fatal(err)
	}
	if stale, err = Run(o(), true); err != nil || len(stale) != 0 {
		t.Fatalf("after the sync, check reports %v (%v)", stale, err)
	}
}

// TestRunReportsEveryStale: check names every stale artifact, not the first,
// and prints the one command.
func TestRunReportsEveryStale(t *testing.T) {
	root := tree(t, "fn (s: Str) len() -> Int\n", "fn (s: Str) old() -> Int\n")
	if err := os.Remove(filepath.Join(root, HabitsStamp)); err != nil {
		t.Fatal(err)
	}
	checkRan := true
	o := Options{Root: root, StdGen: func(root string, check bool) ([]string, error) {
		checkRan = check
		return []string{"std/rt/std_gen_prelude.go"}, nil
	}}
	stale, err := Run(o, true)
	if err != nil {
		t.Fatal(err)
	}
	if !checkRan {
		t.Errorf("StdGen was not told it runs in check mode")
	}
	if len(stale) != 3 {
		t.Fatalf("check reported %v, want the index, the stamp and stdgen's file", stale)
	}
	report := Report(stale)
	for _, want := range []string{SpecPath + " is stale", HabitsStamp, "std/rt/std_gen_prelude.go is stale", Command} {
		if !strings.Contains(report, want) {
			t.Errorf("Report lacks %q:\n%s", want, report)
		}
	}
	if got := strings.Count(report, Command); got != 1 {
		t.Errorf("Report names %s %d times, want once:\n%s", Command, got, report)
	}
	if Report(nil) != "" {
		t.Errorf("Report(nil) is not empty: %q", Report(nil))
	}
}
