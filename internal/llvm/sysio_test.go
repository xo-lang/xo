package llvm_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/build"
	"github.com/xo-lang/xo/internal/gen"
)

// TestSysioParity builds testdata/sysio_parity with the Xo path of the
// files (std/native/llrt/sysio.xo over the OS layer: linux.xo, darwin.xo,
// windows.xo; decisions 0206 and 0213) and with the C path
// (XO_LLRT_C=sysio), runs both, and requires the same output byte for
// byte: each FsErr and each message, which is the C library's strerror
// text on every OS. XO_GOLDEN_WANT_XO=path builds with that xo (xo1)
// instead of the Go driver.
func TestSysioParity(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs a program twice")
	}
	if _, err := build.Clang(); err != nil {
		t.Skip("clang not found: the LLVM backend tests need it")
	}
	dir := filepath.Join("testdata", "sysio_parity")
	xo1 := os.Getenv("XO_GOLDEN_WANT_XO")
	run := func(cpath string) string {
		t.Setenv("XO_LLRT_C", cpath)
		var bin string
		if xo1 != "" {
			b, out, err := xo1Build(xo1, dir, "sysio_parity", build.Host())
			if err != nil {
				t.Fatalf("xo1 build (XO_LLRT_C=%q): %v\n%s", cpath, err, out)
			}
			bin = b
		} else {
			res, err := build.Build([]string{dir}, build.Options{Mode: gen.Build, Backend: build.BackendLLVM})
			if err != nil || res.Failed {
				t.Fatalf("build (XO_LLRT_C=%q): %v %+v", cpath, err, res)
			}
			bin = res.Binaries[build.Host()]
		}
		got, err := runBinary(bin, dir)
		if err != nil || got.exit != 0 {
			t.Fatalf("run (XO_LLRT_C=%q): %v exit %d\n%s\n%s", cpath, err, got.exit, got.stdout, got.stderr)
		}
		return got.stdout
	}
	xo := run("")
	c := run("sysio")
	if xo != c {
		t.Errorf("the Xo path and the C path differ\n--- Xo\n%s\n--- C\n%s", xo, c)
	}
	if !strings.HasSuffix(xo, "done\n") || !strings.Contains(xo, "read nope: NotFound") {
		t.Errorf("unexpected output:\n%s", xo)
	}
	t.Logf("%s/%s: %d lines identical", runtime.GOOS, runtime.GOARCH, strings.Count(xo, "\n"))
}

// TestErrnoTables compares the C library's strerror texts with the
// tables of the macOS and Windows layers (std/native/llrt/darwin.xo,
// windows.xo; decision 0213), which must give the C path's messages byte
// for byte: testdata/errno_probe.c prints the C side.
func TestErrnoTables(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles and runs a C program")
	}
	layer := map[string]string{"darwin": "darwin.xo", "windows": "windows.xo"}[runtime.GOOS]
	if layer == "" {
		t.Skip("the layer of this OS has no table of its own here")
	}
	src, err := os.ReadFile(filepath.Join("..", "..", "std", "native", "llrt", layer))
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "errno_probe")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	if err := build.CompileC(nil, []string{filepath.Join("testdata", "errno_probe.c")}, bin); err != nil {
		t.Fatalf("compile errno_probe.c: %v", err)
	}
	out, err := exec.Command(bin).Output()
	if err != nil {
		t.Fatalf("run errno_probe: %v", err)
	}
	texts, def := strerrorTable(t, string(src))
	for _, line := range strings.Split(strings.TrimRight(strings.ReplaceAll(string(out), "\r\n", "\n"), "\n"), "\n") {
		kind, rest, _ := strings.Cut(line, " ")
		a, b, _ := strings.Cut(rest, " ")
		n, _ := strconv.Atoi(a)
		switch kind {
		case "E":
			want, ok := texts[n]
			if !ok {
				want = strings.ReplaceAll(def, "${e}", a)
			}
			if want != b {
				t.Errorf("strerror(%d): C %q, %s %q", n, b, layer, want)
			}
		}
	}
}

// strerrorTable reads the arms of the layer's `pub fn strerror`: the text
// per errno and the default arm's text.
func strerrorTable(t *testing.T, src string) (map[int]string, string) {
	body := fnBody(t, src, "pub fn strerror(")
	arm := regexp.MustCompile(`^\s+(\d+) => "(.*)"$`)
	def := regexp.MustCompile(`^\s+_ => "(.*)"$`)
	texts := map[int]string{}
	d := ""
	for _, line := range strings.Split(body, "\n") {
		if m := arm.FindStringSubmatch(line); m != nil {
			n, _ := strconv.Atoi(m[1])
			texts[n] = m[2]
		} else if m := def.FindStringSubmatch(line); m != nil {
			d = m[1]
		}
	}
	if len(texts) == 0 || d == "" {
		t.Fatalf("no strerror table")
	}
	return texts, d
}

// fnBody is the text of the function that starts with head, up to its
// closing brace at the start of a line.
func fnBody(t *testing.T, src, head string) string {
	i := strings.Index(src, head)
	if i < 0 {
		t.Fatalf("no %s", head)
	}
	j := strings.Index(src[i:], "\n}\n")
	if j < 0 {
		t.Fatalf("no end of %s", head)
	}
	return src[i : i+j+2]
}
