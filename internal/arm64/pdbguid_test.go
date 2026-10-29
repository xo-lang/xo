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

// TestPDBGUIDFromContent: a windows/amd64 build's bytes (its PDB GUID
// among them) are the same whether separate compilation's facts came
// from the cache's entries (the default) or from an explicit
// XO_SEPCOMP_FACTS directory of copies, and with the back-end cache off.
// The GUID was seeded from the objects' cache keys, which carry cache_id,
// whose facts component differs between the two (Windows run
// 40820335875 after 2d939fabd, 2026-10-29). The PE link runs on the Mac.
func TestPDBGUIDFromContent(t *testing.T) {
	if testing.Short() {
		t.Skip("builds programs")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-pdbguid-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	cache := filepath.Join(work, "cache")
	var store, facts [2]string
	for conc := 0; conc < 2; conc++ {
		cmd := testtmp.Guard(exec.Command(xo, "dev", "stdcache", "--conc="+string(rune('0'+conc)), "--target=windows/amd64"))
		cmd.Env = append(os.Environ(), "XO_BACKEND_CACHE=on", "XO_BACKEND_CACHE_DIR="+cache)
		paths, err := cmd.Output()
		if err != nil {
			t.Fatalf("xo dev stdcache --conc=%d: %v", conc, err)
		}
		lines := strings.Fields(string(paths))
		if len(lines) != 2 {
			t.Fatalf("xo dev stdcache printed %q, want the store and the facts", paths)
		}
		store[conc], facts[conc] = lines[0], lines[1]
	}
	// the facts as an explicit directory of copies: conc0.stdf and
	// conc1.stdf (sepcomp_fact_files' directory form), stamped by
	// cache_id, where the cache's entries are named by their key
	dir := filepath.Join(work, "facts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for conc := 0; conc < 2; conc++ {
		b, err := os.ReadFile(facts[conc])
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "conc"+string(rune('0'+conc))+".stdf"), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	src := "fn main(os: Os) {\n  let words = \"alpha,beta,gamma\".split(\",\")\n  os.stdio.println(\"words ${words.len()} ${words[1].to_upper()}\")\n}\n"
	prog := filepath.Join(work, "prog")
	if err := os.MkdirAll(prog, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(prog, "main.xo"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(prog, "xo.mod"), []byte("module prog\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	build := func(name string, extra ...string) []byte {
		d := filepath.Join(work, name)
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		out := filepath.Join(d, "prog.exe")
		b := testtmp.Guard(exec.Command(xo, "build", "--target=windows/amd64", "-o", out, "."))
		b.Dir = prog
		b.Env = append(append(os.Environ(), "XO_BACKEND_CACHE=on", "XO_BACKEND_CACHE_DIR="+cache, "XO_STD_EXPORT=0"), extra...)
		if msgs, err := b.CombinedOutput(); err != nil {
			t.Fatalf("build %s: %v\n%s", name, err, msgs)
		}
		bin, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		return bin
	}
	explicit := []string{"XO_SEPCOMP=1", "XO_SEPCOMP_FACTS=" + facts[0], "XO_SEPCOMP_FACTS_CONC=" + facts[1],
		"XO_SEPCOMP_STORE=" + store[0], "XO_SEPCOMP_STORE_CONC=" + store[1]}
	entries := build("entries", explicit...)
	copies := build("copies", "XO_SEPCOMP=1", "XO_SEPCOMP_FACTS="+dir, "XO_SEPCOMP_STORE="+store[0], "XO_SEPCOMP_STORE_CONC="+store[1])
	if !bytes.Equal(entries, copies) {
		t.Errorf("the build with the cache's facts entries and the one with a facts directory of copies differ in %d bytes", differingBytes(entries, copies))
	}
	off := build("cacheoff", append(explicit, "XO_BACKEND_CACHE=off")...)
	if !bytes.Equal(entries, off) {
		t.Errorf("the build with the back-end cache on and the one with it off differ in %d bytes", differingBytes(entries, off))
	}
}

func differingBytes(a, b []byte) int {
	n := 0
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			n++
		}
	}
	if len(a) > len(b) {
		return n + len(a) - len(b)
	}
	return n + len(b) - len(a)
}
