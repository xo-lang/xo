package arm64

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestX86FrameProbeLoop requires the function `huge` of
// testdata/run/huge_frame, built for windows/amd64, to probe its frame in
// a loop: the prologue touched each page with its own two instructions,
// and past about 68 KiB a Win64 prologue passed the 255 bytes its
// UNWIND_INFO holds ("a prologue of 365 bytes", a 24k line function,
// 2026-10-22). The run of the program is the Docker amd64 goldens'.
func TestX86FrameProbeLoop(t *testing.T) {
	if testing.Short() {
		t.Skip("lowers a function of 4500 statements")
	}
	asm := winAsm(t, filepath.Join("testdata", "run", "huge_frame"), "windows/amd64", "")
	start := strings.Index(asm, "# func huge\n")
	if start < 0 {
		t.Fatal("no function huge in the assembly")
	}
	body := asm[start:]
	if end := strings.Index(body[1:], "# func "); end > 0 {
		body = body[:end+1]
	}
	if !strings.Contains(body, "\ttestq %r11, (%r11)\n") {
		t.Fatalf("huge does not probe its frame in a loop:\n%.600s", body)
	}
	if n := strings.Count(body, "\tsubq $4096, %rsp\n"); n > 0 {
		t.Fatalf("huge still probes %d pages one by one", n)
	}
	if !strings.Contains(body, "\t.seh_stackalloc ") {
		t.Fatalf("huge records no stack allocation:\n%.600s", body)
	}
}
