package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/gen"
	"github.com/xo-lang/xo/internal/llvm"
)

func TestNativeBackendFor(t *testing.T) {
	cases := []struct{ os, arch, want string }{
		{"darwin", "arm64", BackendArm64},
		{"linux", "arm64", BackendArm64},
		{"linux", "amd64", BackendLLVM},
		{"darwin", "amd64", BackendLLVM},
		{"windows", "arm64", BackendLLVM},
	}
	for _, c := range cases {
		if got := nativeBackendFor(c.os, c.arch); got != c.want {
			t.Errorf("%s/%s: %s, want %s", c.os, c.arch, got, c.want)
		}
	}
}

// noGCDirs are the programs the no GC check is held to: every native
// golden program, the programs of unsupported constructs, the benchmark
// programs, the self hosted lexer, and the examples.
func noGCDirs(t *testing.T) []string {
	t.Helper()
	var dirs []string
	for _, pat := range []string{
		"../llvm/testdata/golden/*",
		"../llvm/testdata/unsupported/*",
		"../../bench/perf/llvm/xo/*",
		"../../selfhost/lexer",
		"../../examples/*",
	} {
		ds, err := filepath.Glob(pat)
		if err != nil {
			t.Fatal(err)
		}
		dirs = append(dirs, ds...)
	}
	if len(dirs) < 70 {
		t.Fatalf("only %d programs found", len(dirs))
	}
	return dirs
}

// TestNoGCCheckAgrees holds the no GC check (decision 0083) to the
// native code generators: for every program, the check reports no
// XO1201 exactly when both the LLVM and the arm64 backend generate code
// for it, and a program the check rejects is rejected by code
// generation too. Code is generated, not compiled, so this also runs
// under -short.
func TestNoGCCheckAgrees(t *testing.T) {
	for _, dir := range noGCDirs(t) {
		dir := dir
		t.Run(filepath.Base(dir), func(t *testing.T) {
			t.Parallel()
			res, ok, err := CheckNative([]string{dir}, false)
			if err != nil || !ok {
				var b strings.Builder
				if res != nil {
					for _, d := range res.Diagnostics {
						b.WriteString(diag.Human(d) + "\n")
					}
				}
				t.Fatalf("check failed: %v\n%s", err, b.String())
			}
			ds := CheckNoGC([]string{dir}, res, gen.Build)
			for _, d := range ds {
				if d.Code != string(diag.NoGCUnavailable) || d.Span.File == "" || d.Span.Line == 0 {
					t.Errorf("not a located XO1201: %s", diag.Human(d))
				}
			}
			root := lineRoot([]string{dir})
			_, llErr := llvm.Generate(res, llvm.Options{LineRoot: root})
			_, armErr := Arm64Asm(res, root, "darwin")
			switch {
			case len(ds) == 0 && (llErr != nil || armErr != nil):
				t.Errorf("the check accepts the program, but code generation fails:\nllvm: %v\narm64: %v", llErr, armErr)
			case len(ds) > 0 && (llErr == nil || armErr == nil):
				t.Errorf("the check rejects the program (%s), but code generation succeeds (llvm %v, arm64 %v)", diag.Human(ds[0]), llErr, armErr)
			}
			if strings.Contains(dir, "unsupported") && len(ds) == 0 {
				t.Errorf("no XO1201 for an unsupported construct")
			}
		})
	}
}

// TestNoGCTestCheck: the no GC check of a test build checks the test
// blocks (a property test is XO1201), and the test list a native test
// build reports is the test blocks in order.
func TestNoGCTestCheck(t *testing.T) {
	dir := t.TempDir()
	src := "fn twice(n: Int) -> Int { n * 2 }\n\ntest \"plain\" {\n  expect(twice(2)) == 4\n}\n\ntest \"property\" for n: Int {\n  expect(twice(n) % 2) == 0\n}\n\ntest \"pinned\" seed 7 uses clock {\n  expect(true)\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "lib_test.xo"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	res, ok, err := CheckNative([]string{dir}, false)
	if err != nil || !ok {
		t.Fatalf("check: %v %v", err, res.Diagnostics)
	}
	ds := CheckNoGC([]string{dir}, res, gen.Test)
	if len(ds) != 1 || !strings.Contains(ds[0].Message, "property test") || ds[0].Span.Line != 7 {
		t.Errorf("want one XO1201 for the property test: %v", ds)
	}
	tis := testInfos([]string{dir}, res)
	if len(tis) != 3 || tis[0].Name != "plain" || tis[2].Seed != 7 || len(tis[2].Uses) != 1 || tis[2].Uses[0] != "clock" || tis[1].Loc != "lib_test.xo:7:1" {
		t.Errorf("tests: %+v", tis)
	}
}

func TestNoGCBackendGo(t *testing.T) {
	_, err := Build([]string{"../llvm/testdata/golden/arith"}, Options{Mode: gen.Build, NoGC: true, Backend: BackendGo})
	if err == nil || !strings.Contains(err.Error(), "native backend") {
		t.Errorf("got %v, want an error naming the native backends", err)
	}
}
