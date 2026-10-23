package arm64

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

// TestUseCDarwinAmd64Links cross builds the use_c_add golden (`use c`,
// linked by cc) for darwin/amd64 with the native amd64 backend and
// requires an x86_64 executable: xo's assembler writes no .s, and the
// link lacked the -U flags for the runtime's weak references (b03, the
// Intel field report on 0.26.0: undefined ___tsan_* and
// ___sanitizer_*_switch_fiber).
func TestUseCDarwinAmd64Links(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("a darwin host's cc links darwin/amd64")
	}
	if testing.Short() {
		t.Skip("builds a program")
	}
	out := filepath.Join(t.TempDir(), "prog")
	res, err := xotool.Build([]string{filepath.Join("..", "llvm", "testdata", "golden", "use_c_add")}, xotool.Options{Backend: "amd64", Targets: "darwin/amd64", Out: out})
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed {
		t.Fatalf("build failed:\n%s", res.Output)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("no binary: %v\n%s", err, res.Output)
	}
	// a 64 bit Mach-O executable for x86_64 (CPU_TYPE_X86_64 0x01000007)
	if len(b) < 8 || b[0] != 0xcf || b[1] != 0xfa || b[4] != 0x07 || b[7] != 0x01 {
		t.Fatalf("not an x86_64 Mach-O: % x", b[:8])
	}
}
