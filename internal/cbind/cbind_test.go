package cbind

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func needClang(t *testing.T) {
	t.Helper()
	if _, err := Clang(); err != nil {
		t.Skip(err)
	}
}

// TestBindTiny binds testdata/tiny/tiny.h and checks the declarations,
// the skipped names with their reasons, and that the adapter compiles
// with the library.
func TestBindTiny(t *testing.T) {
	needClang(t)
	dir := t.TempDir()
	b, err := Bind(Options{Header: "tiny.h", ModDir: "testdata/tiny", Links: []string{"tiny.c"}, Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	decl := string(b.Decl)
	for _, want := range []string{
		"struct TinyCounter {}",
		"struct TinyPoint {\n  x: Int32\n  y: Int32\n}",
		"struct TinyPair {\n  a: Float\n  b: Float32\n  ok: Bool\n}",
		"fn tiny_add(a: Int32, b: Int32) -> Int32 uses ffi",
		"fn tiny_scale(x: Float, f: Float32) -> Float uses ffi",
		"fn tiny_greeting() -> Str uses ffi",
		"fn tiny_count(s: Str) -> UInt64 uses ffi",
		"fn tiny_sum(data: Bytes) -> UInt32 uses ffi",
		"fn tiny_text_len(text: Str) -> Int uses ffi",
		"fn tiny_counter_new(start: Int) -> TinyCounter? uses ffi",
		"fn tiny_counter_get(c: TinyCounter) -> Int uses ffi",
		"fn tiny_is_even(n: Int) -> Bool uses ffi",
		"fn tiny_step(m: Int32, v: Int32) -> Int32 uses ffi",
		"fn tiny_point_add(a: TinyPoint, b: TinyPoint) -> TinyPoint uses ffi",
		"/// C: int tiny_add(int a, int b)\n/// Replay: by value\n",
		"/// Replay: a C handle crosses: re executed by a replay\nfn tiny_counter_inc(c: TinyCounter) uses ffi",
	} {
		if !strings.Contains(decl, want) {
			t.Errorf("declarations lack %q:\n%s", want, decl)
		}
	}
	for name, why := range map[string]string{
		"tiny_out":    "output parameter",
		"tiny_printf": "variadic",
		"tiny_fill":   "mutable buffer",
		"tiny_dup":    "char *",
		"tiny_move":   "which the header defines",
	} {
		if !strings.Contains(b.Skipped[name], why) {
			t.Errorf("skipped[%s] = %q, want it to mention %q", name, b.Skipped[name], why)
		}
	}
	if b.Bound != 19 || b.Total != 24 {
		t.Errorf("coverage %d/%d, want 19/24", b.Bound, b.Total)
	}
	if _, err := os.Stat(b.DeclPath); err != nil {
		t.Errorf("declaration file not written: %v", err)
	}
	// the adapter compiles against the header without warnings (the
	// native build tests link and run it)
	ad := filepath.Join(dir, "adapter.c")
	os.WriteFile(ad, b.Adapter, 0o644)
	clang, _ := Clang()
	out, err := exec.Command(clang, "-Wall", "-Werror", "-c", "-o", filepath.Join(dir, "a.o"), ad).CombinedOutput()
	if err != nil {
		t.Fatalf("adapter does not build: %v\n%s\n%s", err, out, b.Adapter)
	}
}

// TestBindEffects: `uses none` and a declared effect list replace `ffi`.
func TestBindEffects(t *testing.T) {
	needClang(t)
	b, err := Bind(Options{Header: "tiny.h", ModDir: "testdata/tiny", Uses: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b.Decl), "fn tiny_add(a: Int32, b: Int32) -> Int32\n") {
		t.Errorf("uses none: %s", b.Decl)
	}
	b2, err := Bind(Options{Header: "tiny.h", ModDir: "testdata/tiny", Uses: []string{"fs.read", "clock"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b2.Decl), "fn tiny_add(a: Int32, b: Int32) -> Int32 uses fs.read, clock\n") {
		t.Errorf("uses fs.read, clock: %s", b2.Decl)
	}
	if b.Prefix == b2.Prefix {
		t.Errorf("two effect ceilings share the prefix %s", b.Prefix)
	}
}

// TestBindMemoUsesNone: a header bound with `ffi` (Uses nil) and then
// with `uses none` (Uses empty) in one process gets two bindings; the
// memo of Bind took them for one (both print [] with %q), so the second
// `use c` line had the first one's effects.
func TestBindMemoUsesNone(t *testing.T) {
	needClang(t)
	dir := t.TempDir()
	ffi, err := Bind(Options{Header: "tiny.h", ModDir: "testdata/tiny", Links: []string{"tiny.c"}, Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	none, err := Bind(Options{Header: "tiny.h", ModDir: "testdata/tiny", Links: []string{"tiny.c"}, Uses: []string{}, Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if ffi.Key == none.Key || ffi.Prefix == none.Prefix {
		t.Fatalf("ffi and uses none share the binding %s", none.Key)
	}
	if !strings.HasSuffix(none.Key, "|") || !strings.Contains(string(none.Decl), "fn tiny_add(a: Int32, b: Int32) -> Int32\n") {
		t.Errorf("uses none binding: key %s\n%s", none.Key, none.Decl)
	}
}

// TestBindZlib binds a system header when it is installed: zlib's
// crc32 and zlibVersion map, and z_stream functions (a pointer to a
// struct the header defines) are skipped.
func TestBindZlib(t *testing.T) {
	needClang(t)
	b, err := Bind(Options{Header: "zlib.h", ModDir: t.TempDir(), Links: []string{"z"}})
	if err != nil {
		t.Skipf("zlib.h not available: %v", err)
	}
	decl := string(b.Decl)
	for _, want := range []string{"fn zlib_version() -> Str uses ffi", "fn crc32(crc: UInt64, buf: Bytes) -> UInt64 uses ffi"} {
		if !strings.Contains(decl, want) {
			t.Errorf("zlib binding lacks %q:\n%s", want, decl)
		}
	}
	if !strings.Contains(b.Skipped["deflate"], "which the header defines") {
		t.Errorf("deflate: %q", b.Skipped["deflate"])
	}
	if b.Local || len(b.LinkLibs) != 1 || b.LinkLibs[0] != "z" {
		t.Errorf("local %v, libs %v", b.Local, b.LinkLibs)
	}
}

func TestBindErrors(t *testing.T) {
	needClang(t)
	if _, err := Bind(Options{Header: "no_such_header_xo.h", ModDir: "testdata/tiny"}); err == nil {
		t.Error("a missing header binds")
	}
	if _, err := Bind(Options{Header: "tiny.h", ModDir: "testdata/tiny", Links: []string{"missing.c"}}); err == nil || !strings.Contains(err.Error(), "missing.c") {
		t.Errorf("a missing link file: %v", err)
	}
}

func TestNames(t *testing.T) {
	for in, want := range map[string]string{"zlibVersion": "zlib_version", "crc32": "crc32", "deflateInit_": "deflate_init_", "XMLParse": "xml_parse"} {
		if got := snake(in); got != want {
			t.Errorf("snake(%s) = %s, want %s", in, got, want)
		}
	}
	for in, want := range map[string]string{"tiny_point": "TinyPoint", "sqlite3": "Sqlite3", "_x": "X"} {
		if got := upperCamel(in); got != want {
			t.Errorf("upperCamel(%s) = %s, want %s", in, got, want)
		}
	}
	for in, want := range map[string]string{"zlib": "zlib", "my-lib": "my_lib", "SDL": "sdl", "7z": "c_7z"} {
		if got := moduleName(in); got != want {
			t.Errorf("moduleName(%s) = %s, want %s", in, got, want)
		}
	}
}
