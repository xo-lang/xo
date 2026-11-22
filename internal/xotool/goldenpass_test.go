package xotool

import (
	"os"
	"path/filepath"
	"testing"
)

// fakeTB is one golden subtest for GoldenPass.Check: its name, its result,
// and the cleanups Check registers (run by end).
type fakeTB struct {
	testing.TB
	name     string
	failed   bool
	cleanups []func()
}

func (f *fakeTB) Name() string        { return f.name }
func (f *fakeTB) Failed() bool        { return f.failed }
func (f *fakeTB) Skipped() bool       { return false }
func (f *fakeTB) Cleanup(c func())    { f.cleanups = append(f.cleanups, c) }
func (f *fakeTB) Logf(string, ...any) {}
func (f *fakeTB) Helper()             {}
func (f *fakeTB) end(failed bool) {
	f.failed = failed
	for _, c := range f.cleanups {
		c()
	}
}

func writeFile(t *testing.T, p, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestGoldenPassSkipsOnlyPassed: decision 0737 step 2's rule that a skip
// never hides a failure. A golden is skipped only when its binary (build-info
// id masked), its directory's other files and its test name match the key
// of its last passing run; a failing run removes the key, and a run that
// only records (XO_GOLDEN_PASS_SKIP unset) never skips.
func TestGoldenPassSkipsOnlyPassed(t *testing.T) {
	root := t.TempDir()
	store := filepath.Join(root, "store")
	gdir := filepath.Join(root, "golden", "a")
	writeFile(t, filepath.Join(gdir, "main.xo"), "fn main() {}\n")
	writeFile(t, filepath.Join(gdir, "want.txt"), "exit 0\n")
	bin := filepath.Join(root, "a.bin")
	writeFile(t, bin, `code {"id":"0123456789abcdef01234567"} more`)
	binB := filepath.Join(root, "b.bin")
	writeFile(t, binB, "other code")

	// run 1, recording only: A passes, B fails
	p, err := openGoldenPass(store, false)
	if err != nil {
		t.Fatal(err)
	}
	a := &fakeTB{name: "TestGolden/a"}
	if p.Check(a, bin, gdir) {
		t.Fatal("run 1: skipped with no pass.json")
	}
	a.end(false)
	b := &fakeTB{name: "TestGolden/b"}
	if p.Check(b, binB, gdir) {
		t.Fatal("run 1: b skipped with no pass.json")
	}
	b.end(true)
	p.close()
	if s, err := MergeGoldenPass(store); err != nil || s.Passed != 1 || s.Failed != 1 {
		t.Fatalf("merge 1: %+v %v, want 1 passed 1 failed", s, err)
	}
	prev, _ := readPass(store)
	if _, ok := prev["TestGolden/b"]; ok || len(prev) != 1 {
		t.Fatalf("pass.json after run 1: %v, want only TestGolden/a", prev)
	}

	// a record-only run never skips, even with a matching key
	p, _ = openGoldenPass(store, false)
	if p.Check(&fakeTB{name: "TestGolden/a"}, bin, gdir) {
		t.Error("record only: skipped a matching key")
	}

	// run 2, skipping: A's binary differs only in its build-info id
	writeFile(t, bin, `code {"id":"fedcba9876543210fedcba98"} more`)
	p, _ = openGoldenPass(store, true)
	if !p.Check(&fakeTB{name: "TestGolden/a"}, bin, gdir) {
		t.Error("run 2: a not skipped (same binary, id masked)")
	}
	b = &fakeTB{name: "TestGolden/b"}
	if p.Check(b, binB, gdir) {
		t.Error("run 2: b skipped though its last run failed")
	}
	b.end(false)
	if p.Check(&fakeTB{name: "TestGolden/a"}, bin, gdir, "leak=false") {
		t.Error("run 2: skipped with other run settings")
	}
	if p.Check(&fakeTB{name: "TestGoldenRun/a"}, bin, gdir) {
		t.Error("run 2: another test's key skipped")
	}
	writeFile(t, filepath.Join(gdir, "want.txt"), "exit 1\n")
	if p.Check(&fakeTB{name: "TestGolden/a"}, bin, gdir) {
		t.Error("run 2: skipped after want.txt changed")
	}
	writeFile(t, filepath.Join(gdir, "want.txt"), "exit 0\n")
	writeFile(t, bin, `code {"id":"fedcba9876543210fedcba98"} more!`)
	if p.Check(&fakeTB{name: "TestGolden/a"}, bin, gdir) {
		t.Error("run 2: skipped a different binary")
	}
	p.close()
	if s, err := MergeGoldenPass(store); err != nil || s.Skipped != 1 || s.Passed != 1 {
		t.Fatalf("merge 2: %+v %v, want 1 skipped 1 passed", s, err)
	}

	// a run file whose writer still runs (its lock held) is left alone
	held := filepath.Join(store, "run-999999.jsonl")
	writeFile(t, held, `{"test":"TestGolden/a","key":"k","result":"fail"}`+"\n")
	release, err := lockFile(held + ".lock")
	if err != nil {
		t.Fatal(err)
	}
	p.close()
	if s, err := MergeGoldenPass(store); err != nil || s.Failed != 0 {
		t.Fatalf("merge with a held run file: %+v %v, want it left alone", s, err)
	}
	if _, err := os.Stat(held); err != nil {
		t.Fatalf("a held run file was removed: %v", err)
	}
	release()
	os.Remove(held)
	os.Remove(held + ".lock")

	// a pass and a fail of one subtest in one merge (a rerun) drop its key
	p, _ = openGoldenPass(store, false)
	x := &fakeTB{name: "TestGolden/a"}
	p.Check(x, bin, gdir)
	x.end(true)
	p.close()
	p, _ = openGoldenPass(store, false)
	y := &fakeTB{name: "TestGolden/a"}
	p.Check(y, bin, gdir)
	y.end(false)
	p.close()
	if _, err := MergeGoldenPass(store); err != nil {
		t.Fatal(err)
	}
	if prev, _ := readPass(store); prev["TestGolden/a"].Key != "" {
		t.Error("a fail and a pass in one merge kept the key")
	}
	if m, _ := filepath.Glob(filepath.Join(store, "run-*.jsonl")); len(m) != 0 {
		t.Errorf("run files left after the merge: %v", m)
	}
}
