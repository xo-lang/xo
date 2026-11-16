package repotest

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

// TestCheckallExpectedErrors: scripts/checkall.sh (the gate's checkall
// step) lists an error a program has by mistake but not one its line
// expects with `// ERROR XOnnnn` (the errorcheck tests' annotation, which
// their own test checks). A new errorcheck test failed the checkall step
// as new errors before (Xo Lang 3, 2026-11-16, cbor_tag).
// scripts/checkall_test.sh holds the case.
func TestCheckallExpectedErrors(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash")
	}
	r, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	xo := os.Getenv("XO_CLI")
	if xo == "" {
		if xo, err = xotool.Ensure(r, io.Discard); err != nil {
			t.Skipf("no xo: %v", err)
		}
	}
	out, err := exec.Command("bash", filepath.Join(r, "scripts", "checkall_test.sh"), xo).CombinedOutput()
	if err != nil {
		t.Fatalf("checkall_test.sh: %v\n%s", err, out)
	}
}
