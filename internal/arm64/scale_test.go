package arm64

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xo-lang/xo/internal/progen"
)

// TestSwitch65535Arms builds and runs the scale suite's match of 65535
// Int arms (knowledgebase/infrastructure/scale-suite.md): its jump
// table, placed after the function's code, was past adr's 1 MiB reach
// and the assembly failed ("adr out of range", 2026-10-23); a long
// function now keeps its tables after their br (JT_INLINE_WORDS).
func TestSwitch65535Arms(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a match of 65535 arms (about 100 s)")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module scale\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(progen.Scale("switch", 65535)), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := buildRun(t, dir)
	if err != nil {
		t.Fatal(err)
	}
	if want := progen.ScaleWant("switch", 65535); res.exit != 0 || res.stdout != want {
		t.Fatalf("exit %d, stdout %q, want %q\n%s", res.exit, res.stdout, want, res.stderr)
	}
}
