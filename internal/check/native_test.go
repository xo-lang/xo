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

// TestLLRTCPath checks that XO_LLRT_C=text loads the C path of the number
// and time text (std/native/llrt/text_c.xo, decision 0193) in place of
// text.xo, with no diagnostic, that the default is the Xo path, that
// `all` selects every C path, and that an unknown name, or a module
// whose C path decision 0198 deleted (unicode, strings, jsontext), is
// ignored.
func TestLLRTCPath(t *testing.T) {
	dir := t.TempDir()
	src := "fn main(os: Os) {\n  os.stdio.println(\"${Int.parse(\"7\") ?? 0} ${5s} ${'a'.is_letter()} ${\"q\".debug()}\")\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		env, text, unicode, strings, jsontext string
	}{
		{"", "std/native/llrt/text.xo", "std/native/llrt/unicode.xo", "std/native/llrt/strings.xo", "std/native/llrt/jsontext.xo"},
		{"text", "std/native/llrt/text_c.xo", "std/native/llrt/unicode.xo", "std/native/llrt/strings.xo", "std/native/llrt/jsontext.xo"},
		{"strings, jsontext", "std/native/llrt/text.xo", "std/native/llrt/unicode.xo", "std/native/llrt/strings.xo", "std/native/llrt/jsontext.xo"},
		{"all", "std/native/llrt/text_c.xo", "std/native/llrt/unicode.xo", "std/native/llrt/strings.xo", "std/native/llrt/jsontext.xo"},
		{"unicode, nosuch", "std/native/llrt/text.xo", "std/native/llrt/unicode.xo", "std/native/llrt/strings.xo", "std/native/llrt/jsontext.xo"},
	} {
		t.Setenv("XO_LLRT_C", c.env)
		res, err := CheckOpts([]string{dir}, Options{Native: true})
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Diagnostics) > 0 {
			t.Fatalf("XO_LLRT_C=%q: diagnostics:\n%s", c.env, humanAll(res.Diagnostics))
		}
		files := map[string]string{}
		for _, m := range res.Info().AllModules() {
			if len(m.Files) > 0 {
				files[m.Path] = m.Files[0].Path
			}
		}
		if files["native/llrt/text"] != c.text || files["native/llrt/unicode"] != c.unicode || files["native/llrt/strings"] != c.strings {
			t.Errorf("XO_LLRT_C=%q: text from %q, unicode from %q, strings from %q; want %q, %q, %q", c.env, files["native/llrt/text"], files["native/llrt/unicode"], files["native/llrt/strings"], c.text, c.unicode, c.strings)
		}
		// decision 0193 steps 5 and 6: show, report, fspath, and
		// procspec come with the prelude, as does decision 0200's atomic
		for _, name := range []string{"show", "report", "fspath", "procspec", "atomic"} {
			want := "std/native/llrt/" + name + ".xo"
			if c.env == "all" {
				want = "std/native/llrt/" + name + "_c.xo"
			}
			if files["native/llrt/"+name] != want {
				t.Errorf("XO_LLRT_C=%q: %s from %q, want %q", c.env, name, files["native/llrt/"+name], want)
			}
		}
		if files["native/llrt/jsontext"] != c.jsontext {
			t.Errorf("XO_LLRT_C=%q: jsontext from %q, want %q", c.env, files["native/llrt/jsontext"], c.jsontext)
		}
	}
}
