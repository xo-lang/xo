package arm64

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoSharedScratchReload scans the arm64 assembly of programs full of
// cpu.adc pairs and wide field arithmetic (test/codegen/intrinsics: P-256's
// mul4p and sqr4p, AES-GCM, ChaCha20; bench/perf/tls's client: X25519's
// 64 bit field and the TLS stack) for the emitter's scratch register reuse
// (gotchas/arm64-template-scratch-reuse.md): a template's spilled uses are
// reloaded into x16, x17, then x15 for every further one, so two reloads
// into x15 with only loads between them read one value for two operands;
// and two spilled definitions stored from x16 one after the other lost the
// first. Both shapes are silent miscompiles; the adc chains of decision
// 0231's addendum could make them (a chain's template had up to ten uses).
func TestNoSharedScratchReload(t *testing.T) {
	if testing.Short() {
		t.Skip("lowers two large programs")
	}
	for _, dir := range []string{
		filepath.Join("..", "..", "test", "codegen", "intrinsics"),
		filepath.Join("..", "..", "bench", "perf", "tls"),
	} {
		for _, e := range scratchShapes(strings.Split(asm(t, dir, "darwin"), "\n")) {
			t.Errorf("%s: %s", dir, e)
		}
	}
}

// scratchShapes reports TestNoSharedScratchReload's two shapes in the
// lines of a function's assembly.
func scratchShapes(lines []string) []string {
	var out []string
	fn := ""
	for i, l := range lines {
		if strings.HasPrefix(l, "// func ") {
			fn = strings.TrimPrefix(l, "// func ")
		}
		if strings.HasPrefix(l, "\tldr x15, [") {
			// the next non load: a second reload into x15 before it
			for j := i + 1; j < len(lines) && strings.HasPrefix(lines[j], "\tldr "); j++ {
				if strings.HasPrefix(lines[j], "\tldr x15, [") {
					out = append(out, fmt.Sprintf("func %s: two operands reloaded into x15:\n%s", fn, strings.Join(lines[max(0, i-2):min(len(lines), j+3)], "\n")))
					break
				}
			}
		}
		// both into spill slots: a spill store and then a store of the
		// same value elsewhere, its reload dropped (lsr.xo reload_reuse),
		// is no loss
		if strings.HasPrefix(l, "\tstr x16, [sp") && i+1 < len(lines) && strings.HasPrefix(lines[i+1], "\tstr x16, [sp") {
			out = append(out, fmt.Sprintf("func %s: two spilled definitions stored from x16:\n%s", fn, strings.Join(lines[max(0, i-3):min(len(lines), i+3)], "\n")))
		}
	}
	return out
}

// TestScratchShapes: the scan still finds the shapes it was written for
// (gotchas/arm64-template-scratch-reuse.md: two definitions of one
// template stored from x16 into two spill slots, two operands reloaded
// into x15), and not a spill store followed by a store of the same value
// through a pointer (reload_reuse dropped its reload).
func TestScratchShapes(t *testing.T) {
	bad := []string{"// func f", "\tadds x16, x1, x2", "\tcset x16, cs", "\tstr x16, [sp, #8]", "\tstr x16, [sp, #16]"}
	if len(scratchShapes(bad)) != 1 {
		t.Errorf("two spill stores from x16 not found")
	}
	bad2 := []string{"// func f", "\tldr x15, [sp, #8]", "\tldr x15, [sp, #16]", "\tadd x0, x15, x15"}
	if len(scratchShapes(bad2)) != 1 {
		t.Errorf("two reloads into x15 not found")
	}
	ok := []string{"// func f", "\tadd x16, x19, #1", "\tstr x16, [sp, #24]", "\tstr x16, [x28, #8]"}
	if e := scratchShapes(ok); len(e) != 0 {
		t.Errorf("a spill store and a store through a pointer reported: %v", e)
	}
}
