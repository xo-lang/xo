package check

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDumpUnresolvedImport requires the names section of `xo typedump`
// to skip a use of an import that did not resolve (a module xo.mod does
// not require): its *Module is a typed nil, which the name record kept
// and the dump dereferenced (a panic found by the self hosted checker's
// parser mutations, 2026-10-10).
func TestDumpUnresolvedImport(t *testing.T) {
	dir := t.TempDir()
	src := "use example.com/mono/lib\n\nfn f() {\n  let b = lib.Box{v: 5}\n  let n = lib.biggest([3, 8])\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Check([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Diagnostics) == 0 {
		t.Fatal("no diagnostic for a module xo.mod does not require")
	}
	dump := res.Dump(Sections{Names: true, Types: true, Sigs: true})
	if strings.Contains(dump, " lib module") {
		t.Fatalf("the unresolved module has a name record:\n%s", dump)
	}
}
