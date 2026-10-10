package arm64_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/build"
	"github.com/xo-lang/xo/internal/gen"
)

// TestBigFrameProbes requires the function `big` of testdata/run/big_frame
// to get a frame of more than two pages (it failed with "frame too large"
// past 4000 bytes before) whose prologue touches every page it skips
// (gotchas/arm64-large-frame.md). TestRun runs the program, on the main
// stack and on a fiber.
func TestBigFrameProbes(t *testing.T) {
	asm := linuxAsm(t, "big_frame")
	start := strings.Index(asm, "// func big\n")
	if start < 0 {
		t.Fatal("no function big in the assembly")
	}
	body := asm[start:]
	if end := strings.Index(body[1:], "// func "); end > 0 {
		body = body[:end+1]
	}
	if n := strings.Count(body, "\tsub sp, sp, #1, lsl #12\n\tstr xzr, [sp]\n"); n < 2 {
		t.Fatalf("big probes %d pages, want at least 2:\n%.600s", n, body)
	}
	if !strings.Contains(body, "add x29, sp, #") || !strings.Contains(body, ", lsl #12\n") {
		t.Fatalf("big sets its frame pointer past 4 KiB without a shifted add:\n%.600s", body)
	}
}

// TestWideHelperRecords requires the reference counting helpers of the
// 80 word values of testdata/run/wide_values to store their frame record
// with two str past the 504 byte reach of stp (the assembler rejected
// `stp x29, x30, [sp, #688]` before). TestRun runs the program.
func TestWideHelperRecords(t *testing.T) {
	asm := linuxAsm(t, "wide_values")
	if !strings.Contains(asm, "\tstr x29, [sp, #") || !strings.Contains(asm, "\tldr x30, [sp, #") {
		t.Fatal("no helper stores its frame record past 504 bytes")
	}
}

// linuxAsm builds testdata/run/<name> for linux/arm64 and returns the
// assembly.
func linuxAsm(t *testing.T, name string) string {
	t.Helper()
	lin := build.Target{OS: "linux", Arch: "arm64"}
	res, err := build.Build([]string{filepath.Join("testdata", "run", name)}, build.Options{Mode: gen.Build, Backend: build.BackendArm64, Targets: []build.Target{lin}})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(res.Asm[lin]) // the path of the assembly file
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
