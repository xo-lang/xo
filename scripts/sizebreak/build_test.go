package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

// TestSizebreakOnBuiltProgram builds a one line program with the arm64
// backend and measures it: the symbol gaps must cover __text exactly, the
// module groups must exist, and the reachability scan must find the entry
// and leave almost nothing unreachable in so small a program.
func TestSizebreakOnBuiltProgram(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a program with xo")
	}
	if runtime.GOARCH != "arm64" {
		t.Skip("the native arm64 backend builds this host only on arm64")
	}
	dir := t.TempDir()
	src := "fn main(os: Os) {\n  os.stdio.println(\"hi\")\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	mod := "module github.com/xo-lang/xo/sizebreaktest\nedition preview\n"
	if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte(mod), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := xotool.Build([]string{dir}, xotool.Options{Backend: xotool.BackendArm64})
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed {
		t.Fatalf("build failed: %s", res.Output)
	}
	if res.Dir != "" {
		defer os.RemoveAll(res.Dir)
	}
	path := res.Binaries[xotool.Host()]
	if path == "" {
		t.Fatalf("no binary for %s: %s", xotool.Host(), res.Output)
	}
	bin, err := loadBinary(path)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := analyze(bin)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Sum != rep.Text.Size || rep.Text.Size == 0 {
		t.Fatalf("sum %d, text %d", rep.Sum, rep.Text.Size)
	}
	if rep.Symbols == 0 || len(rep.Groups) == 0 || len(rep.Top) == 0 {
		t.Fatalf("empty report: %+v", rep)
	}
	seen := map[string]bool{}
	for _, g := range rep.Groups {
		seen[g.Name] = true
	}
	for _, want := range []string{"main", NonXo} {
		if !seen[want] {
			t.Errorf("group %q missing from %v", want, rep.Groups)
		}
	}
	r := reachability(bin)
	if !r.Supported {
		t.Fatalf("reachability not supported: %s", r.Note)
	}
	if r.Reachable == 0 || r.Symbols == 0 {
		t.Fatalf("reachability: %+v", r)
	}
}
