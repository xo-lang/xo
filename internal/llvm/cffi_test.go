package llvm_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/build"
	"github.com/xo-lang/xo/internal/gen"
)

// cffiMain calls every function of internal/cbind's tiny library that
// binds: scalars of each width, Float32, Bool, enums, Str in and out,
// Str and Bytes with a length, handles with a NULL result, and flat
// structs in and out.
const cffiMain = `use c "tiny.h" link "%s"

fn main(os: Os) {
  os.stdio.println("add ${tiny.tiny_add(2, 40)}")
  os.stdio.println("scale ${tiny.tiny_scale(1.5, 2.0)}")
  os.stdio.println("greet ${tiny.tiny_greeting()}")
  os.stdio.println("count ${tiny.tiny_count("hello")}")
  os.stdio.println("sum ${tiny.tiny_sum(b"abc")}")
  os.stdio.println("text ${tiny.tiny_text_len("héllo")}")
  let msg = "round " + "trip"
  os.stdio.println("echo ${tiny.tiny_echo(msg)}")
  if let Some(c) = tiny.tiny_counter_new(41) {
    tiny.tiny_counter_inc(c)
    os.stdio.println("counter ${tiny.tiny_counter_get(c)}")
    tiny.tiny_counter_free(c)
  }
  os.stdio.println("none ${tiny.tiny_counter_none().is_none()}")
  os.stdio.println("even ${tiny.tiny_is_even(10)} ${tiny.tiny_is_even(7)}")
  os.stdio.println("big ${tiny.tiny_big()} neg ${tiny.tiny_neg8()} byte ${tiny.tiny_byte(255)}")
  os.stdio.println("step ${tiny.tiny_step(0, 5)} ${tiny.tiny_step(1, 5)}")
  let p = tiny.tiny_point_add(tiny.TinyPoint{x: 1, y: 2}, tiny.TinyPoint{x: 10, y: -20})
  os.stdio.println("point ${p.x} ${p.y}")
  let q = tiny.tiny_pair_make(2.5, 0.5)
  os.stdio.println("pair ${q.a} ${q.b} ${q.ok}")
}
`

const cffiWant = `add 42
scale 3.0
greet hello from C
count 5
sum 294
text 6
echo round trip
counter 42
none true
even true false
big 18446744073709551615 neg -5 byte 0
step 6 4
point 11 -18
pair 2.5 0.5 true
`

// TestUseC builds a program that calls a tiny C library (decision 0116)
// on every native backend, once linking the library as an archive built
// here with clang and ar, once as its C source, and requires the same
// output and no allocation live at exit.
func TestUseC(t *testing.T) {
	clang, err := build.Clang()
	if err != nil {
		t.Skip("clang not found: the LLVM backend tests need it")
	}
	if testing.Short() {
		t.Skip("builds four native programs")
	}
	src := filepath.Join("..", "cbind", "testdata", "tiny")
	for _, form := range []string{"archive", "source"} {
		form := form
		t.Run(form, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			for _, f := range []string{"tiny.h", "tiny.c"} {
				data, err := os.ReadFile(filepath.Join(src, f))
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, f), data, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			link := "tiny.c"
			if form == "archive" {
				link = "libtiny.a"
				if out, err := exec.Command(clang, "-c", "-O1", "-o", filepath.Join(dir, "tiny.o"), filepath.Join(dir, "tiny.c")).CombinedOutput(); err != nil {
					t.Fatalf("compiling the library: %v\n%s", err, out)
				}
				ar, err := exec.LookPath("ar")
				if err != nil {
					t.Skip("ar not found")
				}
				if out, err := exec.Command(ar, "rcs", filepath.Join(dir, "libtiny.a"), filepath.Join(dir, "tiny.o")).CombinedOutput(); err != nil {
					t.Fatalf("ar: %v\n%s", err, out)
				}
				os.Remove(filepath.Join(dir, "tiny.c"))
			}
			main := strings.Replace(cffiMain, "%s", link, 1)
			if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(main), 0o644); err != nil {
				t.Fatal(err)
			}
			for _, be := range nativeBackends() {
				res, b := buildRun(t, dir, build.Options{Mode: gen.Build, Backend: be})
				if res.exit != 0 || res.stdout != cffiWant {
					t.Errorf("%s: %s\nwant stdout:\n%s", be, res, cffiWant)
				}
				leakCheck(t, b.Binaries[build.Host()], dir)
			}
		})
	}
}

// TestUseCZlib calls the system zlib when its header and library are
// installed: `uses none` (a human vouched that crc32 and adler32
// perform no effect), Bytes with a length, and a Str result.
func TestUseCZlib(t *testing.T) {
	if _, err := build.Clang(); err != nil {
		t.Skip("clang not found: the LLVM backend tests need it")
	}
	if testing.Short() {
		t.Skip("builds native programs")
	}
	dir := t.TempDir()
	main := "use c \"zlib.h\" link \"z\" uses none\n\npub fn checksums(data: Bytes) -> (UInt64, UInt64) {\n  (zlib.crc32(0, data), zlib.adler32(1, data))\n}\n\nfn main(os: Os) {\n  let (c, a) = checksums(b\"hello world\")\n  os.stdio.println(\"crc32 ${c} adler32 ${a} version ${zlib.zlib_version().len() > 0}\")\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}
	res, ok, err := build.CheckNative([]string{dir}, false)
	if err != nil || !ok {
		for _, d := range res.Diagnostics {
			if d.Code == "XO1104" {
				t.Skipf("zlib.h not available: %s", d.Message)
			}
		}
		t.Fatalf("check: %v %v", err, res.Diagnostics)
	}
	for _, be := range nativeBackends() {
		r := buildAndRun(t, dir, build.Options{Mode: gen.Build, Backend: be})
		if want := "crc32 222957957 adler32 436929629 version true\n"; r.exit != 0 || r.stdout != want {
			t.Errorf("%s: %s\nwant %s", be, r, want)
		}
	}
}
