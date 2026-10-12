package check

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/std"
)

// TestNativeModules checks the implementation modules of the native
// backends (std/native, decision 0074) through a program that uses every
// std module that has one: they must load and produce no diagnostic, and
// a user module cannot import their intrinsics.
func TestNativeModules(t *testing.T) {
	entries, err := fs.ReadDir(std.NativeFiles, "native")
	if err != nil {
		t.Fatal(err)
	}
	var uses []string
	for _, e := range entries {
		if e.IsDir() {
			// native/llrt: the runtime modules (decision 0180)
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".xo")
		if name == "prelude" || strings.HasPrefix(name, "rt") {
			continue
		}
		uses = append(uses, "use std/"+name)
	}
	dir := t.TempDir()
	src := strings.Join(uses, "\n") + "\n\nfn main(os: Os) {\n  os.stdio.println(\"hi\")\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := CheckOpts([]string{dir}, Options{Native: true})
	if err != nil {
		t.Fatal(err)
	}
	var native []string
	for _, d := range res.Diagnostics {
		if strings.Contains(d.Span.File, "std/native") {
			native = append(native, d.Message)
		}
	}
	if len(native) > 0 {
		t.Errorf("diagnostics in the native modules:\n%s", humanAll(res.Diagnostics))
	}
	loaded := map[string]bool{}
	for _, m := range res.Info().AllModules() {
		if m.Native {
			loaded[m.Implements] = true
		}
	}
	if !loaded["std/prelude"] {
		t.Errorf("native/prelude not loaded: %v", loaded)
	}
	for _, u := range uses {
		if !loaded[strings.TrimPrefix(u, "use ")] {
			t.Errorf("no native module for %s", u)
		}
	}
	// the intrinsics are not visible to user code
	ds := checkSrc(t, "use native/rt\n\nfn main(os: Os) {\n  os.stdio.println(\"${rt.str_byte(\"a\", 0)}\")\n}\n")
	if len(ds) == 0 || !strings.Contains(humanAll(ds), "not found") {
		t.Errorf("user import of native/rt: want module not found, got:\n%s", humanAll(ds))
	}
	// nor are the runtime modules written in Xo (decision 0180), which the
	// prelude and std/regexp load
	ds = checkSrc(t, "use native/llrt/unicode\n\nfn main(os: Os) {\n  os.stdio.println(\"${unicode.is_letter(97)}\")\n}\n")
	if len(ds) == 0 || !strings.Contains(humanAll(ds), "not found") {
		t.Errorf("user import of native/llrt/unicode: want module not found, got:\n%s", humanAll(ds))
	}
	llrt := false
	for _, m := range res.Info().AllModules() {
		llrt = llrt || (m.Path == "native/llrt/unicode" && m.Native)
	}
	if !llrt {
		t.Errorf("native/llrt/unicode not loaded")
	}
}
