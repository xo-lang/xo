package build

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/gen"
)

// TestParseTargetAttrs reads the target of clang's cc1 line as `clang
// -###` prints it on macOS and on Linux (decision 0119).
func TestParseTargetAttrs(t *testing.T) {
	cases := []struct{ out, want string }{
		{
			`Apple clang version 21.0.0
 "/usr/bin/clang" "-cc1" "-triple" "arm64-apple-macosx27.0.0" "-target-cpu" "apple-m1" "-target-feature" "+v8.5a" "-target-feature" "+lse" "-o" "/dev/null"`,
			`"target-cpu"="apple-m1" "target-features"="+v8.5a,+lse"`,
		},
		{
			`Debian clang version 19.1.7
 "/usr/lib/llvm-19/bin/clang" "-cc1" "-triple" "aarch64-unknown-linux-gnu" "-target-cpu" "generic" "-target-feature" "+v8a" "-target-feature" "+fp-armv8" "-target-feature" "+neon" "-target-feature" "+outline-atomics"`,
			`"target-cpu"="generic" "target-features"="+v8a,+fp-armv8,+neon,+outline-atomics"`,
		},
		{"clang: error: no input files\n", ""},
	}
	for _, c := range cases {
		if got := parseTargetAttrs(c.out); got != c.want {
			t.Errorf("parseTargetAttrs:\n got %s\nwant %s", got, c.want)
		}
	}
}

// TestLLVMTargetAttrs builds a program with the LLVM backend and checks
// that every generated function definition carries this clang's target
// (and so, on arm64, compiles shared counts to LSE or outline atomics).
func TestLLVMTargetAttrs(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a native program")
	}
	clang, err := Clang()
	if err != nil {
		t.Skip("clang not found")
	}
	attrs := ClangTargetAttrs(clang)
	t.Logf("%s: %s", clang, attrs)
	if !strings.Contains(attrs, `"target-cpu"=`) {
		t.Fatalf("ClangTargetAttrs(%s) = %q: no target CPU", clang, attrs)
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module example.com/t\nedition 2026\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "main.xo"), []byte("fn main(os: Os) {\n  let n = Atomic[Int].new(1)\n  os.stdio.println(\"${n.get()}\")\n}\n"), 0o644)
	t.Setenv("XO_BUILD_DIR", filepath.Join(dir, "build"))
	res, err := Build([]string{dir}, Options{Mode: gen.Build, Backend: BackendLLVM, Out: filepath.Join(dir, "prog")})
	if err != nil {
		t.Fatal(err)
	}
	ir, err := os.ReadFile(filepath.Join(res.Dir, "prog.ll"))
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range []string{"#1", "#4"} {
		re := regexp.MustCompile(`attributes ` + g + ` = \{[^}]*` + regexp.QuoteMeta(attrs) + ` \}`)
		if !re.Match(ir) {
			t.Errorf("attribute group %s lacks %s", g, attrs)
		}
	}
}
