package llvm

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

// TestLLVMAdcChain checks the machine code LLVM makes of cpu.adc and
// adc_carry (decision 0231's addendum): P-256's field product
// (nistec.mul4p, its sums chains of adc pairs) disassembled from the
// test/codegen/intrinsics program must be adcs chains, not a compare and
// a cset per carry. When this was written (2026-10-16) it was 153
// instructions with 32 adcs, against 232 with 24 adc and 38 cmn when
// each carry was computed apart from its sum.
func TestLLVMAdcChain(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a program")
	}
	if runtime.GOARCH != "arm64" {
		t.Skip("adcs is arm64's")
	}
	if _, err := xotool.Clang(); err != nil {
		t.Skip("clang not found")
	}
	objdump, err := exec.LookPath("objdump")
	if err != nil {
		t.Skip("objdump not found")
	}
	dir := filepath.Join("..", "..", "test", "codegen", "intrinsics")
	res, err := xotool.Build([]string{dir}, xotool.Options{Backend: xotool.BackendLLVM, NoGC: true, Out: filepath.Join(t.TempDir(), "intr")})
	if err != nil || res.Failed {
		t.Fatalf("build: %v %v", err, res.Output)
	}
	out, err := exec.Command(objdump, "-d", "--no-show-raw-insn", res.Binaries[xotool.Host()]).Output()
	if err != nil {
		t.Fatalf("objdump: %v", err)
	}
	var body []string
	in := false
	for _, l := range strings.Split(string(out), "\n") {
		if strings.HasSuffix(l, "<x.nistec.mul4p>:") || strings.HasSuffix(l, "<_x.nistec.mul4p>:") {
			in = true
			continue
		}
		if in {
			if strings.TrimSpace(l) == "" {
				break
			}
			body = append(body, l)
		}
	}
	if len(body) == 0 {
		t.Fatal("no nistec.mul4p in the binary (inlined everywhere?)")
	}
	adcs := 0
	for _, l := range body {
		f := strings.Fields(l)
		if len(f) >= 2 && f[1] == "adcs" {
			adcs++
		}
	}
	if adcs < 24 || len(body) > 170 {
		t.Errorf("nistec.mul4p: %d instructions, %d adcs; want at most 170 and at least 24\n%s", len(body), adcs, strings.Join(body, "\n"))
	}
}
