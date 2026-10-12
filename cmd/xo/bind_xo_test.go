package main

import (
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// xoBindBinary builds selfhost/cli/bind/xobind (`xo bind` in Xo, decision
// 0178 stage 6) natively, or returns $XO_BIND_BIN.
func xoBindBinary(t *testing.T) string {
	t.Helper()
	if b := os.Getenv("XO_BIND_BIN"); b != "" {
		return b
	}
	bin := filepath.Join(t.TempDir(), "xobind")
	t.Setenv("XO_BUILD_DIR", filepath.Join(t.TempDir(), "build"))
	var out, errOut bytes.Buffer
	if code := runXo([]string{"build", "--backend=arm64", "-o", bin, filepath.Join("..", "..", "selfhost", "cli", "bind", "xobind")}, &out, &errOut); code != 0 {
		t.Fatalf("building xobind: exit %d\n%s%s", code, out.String(), errOut.String())
	}
	return bin
}

// cbindTemp is the temporary directory of clang's translation unit, which
// clang's error messages name.
var cbindTemp = regexp.MustCompile(`xo-cbind-[A-Za-z0-9]+`)

// TestBindCXoSame runs `xo bind --c` with internal/cbind (in process) and
// with the C binder in Xo (xobind) over the `use c` headers of the
// repository, zlib.h from the include path, and the error cases, in the
// same directory one after the other, and requires the same exit status,
// standard output and error (clang's temporary file name normalized),
// and the same files in the binding cache: declaration files and
// adapters byte identical (decision 0178 stage 6).
func TestBindCXoSame(t *testing.T) {
	if _, err := exec.LookPath("clang"); err != nil && os.Getenv("XO_CLANG") == "" {
		t.Skip("clang not found")
	}
	if testing.Short() && os.Getenv("XO_BIND_BIN") == "" {
		t.Skip("builds xobind natively; set XO_BIND_BIN to run it with -short")
	}
	bin := xoBindBinary(t)
	t.Setenv("XO_CBIND_DIR", "")
	repo, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	two := map[string]string{
		"two.h": "int two_add(int a, int b);\nvoid two_out(int *p);\n",
		"two.c": "#include \"two.h\"\nint two_add(int a, int b) { return a + b; }\nvoid two_out(int *p) { *p = 2; }\n",
	}
	odd := map[string]string{
		// keywords, names in camel case, typedef chains, a union, an
		// enum, bytes and a length, const pointers, anonymous structs
		"odd.h": `#include <stddef.h>
#include <stdint.h>
typedef unsigned long my_size;
typedef my_size my_len;
typedef struct opaque_t *opaque_ref;
union num { int i; float f; };
struct box { int64_t w; uint8_t h; };
struct nested { struct box b; };
enum color { RED, GREEN };
typedef struct { int x; } anon_t;
int matchFn(int type, int fn);
int HTTPServerPort(void);
int64_t Sum2Bytes(const uint8_t *data, my_len count);
const char *echoText(const char *text, size_t text_len);
opaque_ref opaque_new(void);
void opaque_free(opaque_ref r);
struct box box_make(int64_t w, uint8_t h);
int nested_w(struct nested n);
int union_i(union num u);
enum color next_color(enum color c);
anon_t anon_make(void);
void cb(void (*f)(int));
int arr(int a[4]);
_Atomic int atomic_get(void);
volatile int vol(void);
long double ld(void);
_Bool b(_Bool x);
int _reserved(void);
int dup1(int a, int a2, int A);
int self(int static_);
`,
	}
	cases := []struct {
		name  string
		src   string            // a directory of the repository to copy, or ""
		files map[string]string // files written instead
		args  []string
	}{
		{"tiny", "internal/cbind/testdata/tiny", nil, []string{"--link", "tiny.c", "tiny.h", "no_such_xo.h"}},
		{"tiny-json-none", "internal/cbind/testdata/tiny", nil, []string{"--json", "--uses", "none", "--link", "tiny.c", "tiny.h", "no_such_xo.h"}},
		{"tiny-uses", "internal/cbind/testdata/tiny", nil, []string{"--json", "--uses", "fs.read, clock", "tiny.h"}},
		{"stage6c", "internal/selfhost/testdata/ssa/stage6c", nil, []string{"--link", "tiny.c", "tiny.h"}},
		{"stage6c-skipped", "internal/selfhost/testdata/ssa/stage6c_skipped", nil, []string{"--json", "--link", "tiny.c", "tiny.h"}},
		{"rtclock", "examples/realtime-control", nil, []string{"--uses", "none", "--link", "rtclock.c", "rtclock.h"}},
		{"zlib", "", map[string]string{"xo.mod": "module ex\n"}, []string{"--json", "--link", "z", "zlib.h"}},
		{"zlib-text", "", map[string]string{"xo.mod": "module ex\n"}, []string{"--link", "z", "zlib.h"}},
		{"two", "", two, []string{"--link", "two.c", "two.h"}},
		{"odd", "", odd, []string{"--json", "odd.h"}},
		{"odd-text", "", odd, []string{"odd.h"}},
		{"missing-link", "", two, []string{"--json", "--link", "missing.c", "two.h"}},
		{"missing-link-text", "", two, []string{"--link", "missing.c", "--link", "m", "two.h"}},
		{"no-headers", "", two, nil},
		{"bad-flag", "", two, []string{"--bogus", "two.h"}},
		{"help", "", two, []string{"-h"}},
		{"abs-header", "", two, []string{"--json", "ABS/two.h"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "app")
			setup := func() {
				os.RemoveAll(dir)
				if tc.src != "" {
					copyTree(t, filepath.Join(repo, tc.src), dir)
				}
				for name, src := range tc.files {
					p := filepath.Join(dir, name)
					os.MkdirAll(filepath.Dir(p), 0o755)
					if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
						t.Fatal(err)
					}
				}
			}
			args := []string{"bind", "--c"}
			for _, a := range tc.args {
				args = append(args, strings.ReplaceAll(a, "ABS", dir))
			}
			var got [2]string
			var trees [2]map[string]string
			for k := 0; k < 2; k++ {
				setup()
				t.Chdir(dir)
				var out, errOut bytes.Buffer
				var code int
				if k == 0 {
					code = runXo(args, &out, &errOut)
				} else {
					code = execCLI([]string{bin}, args, &out, &errOut)
				}
				got[k] = cbindTemp.ReplaceAllString("exit "+strconv.Itoa(code)+"\n--- stdout\n"+out.String()+"--- stderr\n"+errOut.String(), "xo-cbind-TMP")
				trees[k] = readTree(t, dir)
			}
			if got[0] != got[1] {
				t.Errorf("output differs\n=== Go\n%s\n=== Xo\n%s", got[0], got[1])
			}
			if len(trees[0]) != len(trees[1]) {
				t.Errorf("files: Go %v, Xo %v", keys(trees[0]), keys(trees[1]))
			}
			for name, data := range trees[0] {
				if x, ok := trees[1][name]; !ok {
					t.Errorf("%s: written by Go only", name)
				} else if x != data {
					t.Errorf("%s differs\n=== Go\n%s\n=== Xo\n%s", name, data, x)
				}
			}
		})
	}
}

func copyTree(t *testing.T, from, to string) {
	t.Helper()
	err := filepath.WalkDir(from, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(from, p)
		dst := filepath.Join(to, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// readTree is every file below dir by slash path.
func readTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		data, _ := os.ReadFile(p)
		rel, _ := filepath.Rel(dir, p)
		out[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	return out
}

func keys(m map[string]string) []string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
