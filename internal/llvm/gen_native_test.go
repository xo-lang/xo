package llvm_test

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/build"
	"github.com/xo-lang/xo/internal/gen"
)

// TestGenGoldensNative builds the programs of internal/gen's goldens
// listed in testdata/gen_native.txt with the Go backend, the LLVM
// backend, and (on an arm64 host) the arm64 backend, all without a
// garbage collector for the native ones, and requires the same exit
// status, stdout, and fault messages (stableStderr), and no allocation
// live at exit for programs that exit normally. The list holds every Go
// golden the native backends build whose output does not depend on the
// clock, the network, or the machine; a program that stops building
// natively (XO1201) fails the test.
func TestGenGoldensNative(t *testing.T) {
	if _, err := build.Clang(); err != nil {
		t.Skip("clang not found: the LLVM backend tests need it")
	}
	if testing.Short() {
		t.Skip("builds every listed program three times")
	}
	names := genNativeList(t)
	backends := []string{build.BackendLLVM}
	if runtime.GOARCH == "arm64" && (runtime.GOOS == "darwin" || runtime.GOOS == "linux") {
		backends = append(backends, build.BackendArm64)
	}
	for _, name := range names {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := filepath.Join("..", "gen", "testdata", "golden", name)
			goRes := buildAndRun(t, dir, build.Options{Mode: gen.Build, Backend: build.BackendGo})
			for _, be := range backends {
				res, b := buildRun(t, dir, build.Options{Mode: gen.Build, Backend: be})
				if goRes.String() != res.String() {
					t.Errorf("%s backend differs from the Go backend\n--- go\n%s\n--- %s\n%s", be, goRes, be, res)
				}
				if res.exit == 0 {
					leakCheck(t, b.Binaries[build.Host()], dir)
				}
			}
		})
	}
}

// genNativeList reads testdata/gen_native.txt: one golden name per line,
// `#` comments.
func genNativeList(t *testing.T) []string {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "gen_native.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if i := strings.IndexByte(l, '#'); i >= 0 {
			l = strings.TrimSpace(l[:i])
		}
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}
