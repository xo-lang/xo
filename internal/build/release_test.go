package build

import (
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/llvm"
)

// TestReleaseNative checks that the native backends compile out debug
// contracts in a release build (core 8) like the Go backend
// (internal/gen TestGoldenRelease runs the same program there). Code is
// generated, not compiled, so this runs on every host.
func TestReleaseNative(t *testing.T) {
	dir := "../gen/testdata/release/debug_contracts"
	res, ok, err := CheckNative([]string{dir}, false)
	if err != nil || !ok {
		t.Fatalf("check: %v", err)
	}
	debugOnly := []string{"requires failed: n % 2 == 0", "ensures failed: result > 100"}
	always := []string{"requires failed: n >= 0", "ensures failed: result > n"}
	for _, release := range []bool{false, true} {
		prog, err := llvm.Generate(res, llvm.Options{LineRoot: lineRoot([]string{dir}), Release: release})
		if err != nil {
			t.Fatalf("release=%v: %v", release, err)
		}
		ir := string(prog.IR)
		for _, s := range always {
			if !strings.Contains(ir, s) {
				t.Errorf("release=%v: %q missing", release, s)
			}
		}
		for _, s := range debugOnly {
			if strings.Contains(ir, s) == release {
				t.Errorf("release=%v: %q present=%v", release, s, !release)
			}
		}
	}
}
