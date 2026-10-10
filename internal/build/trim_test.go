package build

import (
	"bytes"
	"debug/dwarf"
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"os"
	"path/filepath"
	"testing"

	"github.com/xo-lang/xo/internal/gen"
)

// binDWARF returns the DWARF data of the executable at path, or an error
// when it has none.
func binDWARF(t *testing.T, path string) (*dwarf.Data, error) {
	t.Helper()
	if f, err := elf.Open(path); err == nil {
		defer f.Close()
		return f.DWARF()
	}
	if f, err := macho.Open(path); err == nil {
		defer f.Close()
		return f.DWARF()
	}
	f, err := pe.Open(path)
	if err != nil {
		t.Fatalf("%s: not an ELF, Mach-O, or PE executable", path)
	}
	defer f.Close()
	return f.DWARF()
}

// TestGoBinaryTrimming checks decision 0098 on the Go backend: binaries
// carry no DWARF unless XO_GO_DWARF=1, and a program that does not import
// std/http does not link net/http (rt built with xo_nohttp). Function
// names stay in the pclntab of a stripped binary, so net/http's show up
// there when it is linked (the HTTP program checks that the probe works).
func TestGoBinaryTrimming(t *testing.T) {
	if testing.Short() {
		t.Skip("builds three Go programs")
	}
	dir := t.TempDir()
	write := func(name, src string) string {
		d := filepath.Join(dir, name)
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "main.xo"), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		return d
	}
	hello := write("hello", "fn main(os: Os) {\n  os.stdio.println(\"hi\")\n}\n")
	web := write("web", "use std/http\n\nfn main(os: Os) {\n  let r = http.new_request(\"GET\", \"/x\")\n  os.stdio.println(r.path)\n}\n")
	build := func(src, name string) []byte {
		t.Helper()
		out := filepath.Join(dir, "out", name)
		res, err := Build([]string{src}, Options{Mode: gen.Build, Backend: BackendGo, Out: out})
		if err != nil || res.Failed {
			t.Fatalf("build %s: %v %+v", name, err, res)
		}
		data, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := binDWARF(t, out); err == nil {
			if !GoDWARF() {
				t.Errorf("%s: the binary carries DWARF without XO_GO_DWARF=1", name)
			}
		} else if GoDWARF() {
			t.Errorf("%s: no DWARF with XO_GO_DWARF=1: %v", name, err)
		}
		return data
	}
	t.Setenv("XO_GO_DWARF", "")
	if bin := build(hello, "hello"); bytes.Contains(bin, []byte("net/http.")) {
		t.Errorf("hello world links net/http")
	}
	if bin := build(web, "web"); !bytes.Contains(bin, []byte("net/http.")) {
		t.Errorf("a program using std/http does not link net/http (the probe is broken)")
	}
	t.Setenv("XO_GO_DWARF", "1")
	build(hello, "hello-dwarf")
}
