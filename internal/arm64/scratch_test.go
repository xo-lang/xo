package arm64

import (
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
		lines := strings.Split(asm(t, dir, "darwin"), "\n")
		fn := ""
		for i, l := range lines {
			if strings.HasPrefix(l, "// func ") {
				fn = strings.TrimPrefix(l, "// func ")
			}
			if strings.HasPrefix(l, "\tldr x15, [") {
				// the next non load: a second reload into x15 before it
				for j := i + 1; j < len(lines) && strings.HasPrefix(lines[j], "\tldr "); j++ {
					if strings.HasPrefix(lines[j], "\tldr x15, [") {
						t.Errorf("%s: func %s: two operands reloaded into x15:\n%s", dir, fn, strings.Join(lines[max(0, i-2):min(len(lines), j+3)], "\n"))
						break
					}
				}
			}
			if strings.HasPrefix(l, "\tstr x16, [") && i+1 < len(lines) && strings.HasPrefix(lines[i+1], "\tstr x16, [") {
				t.Errorf("%s: func %s: two spilled definitions stored from x16:\n%s", dir, fn, strings.Join(lines[max(0, i-3):min(len(lines), i+3)], "\n"))
			}
		}
	}
}
