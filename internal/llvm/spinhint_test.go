package llvm

import "testing"

// TestSpinHintAsm checks rt.spin_hint's lowering (decision 0200): the
// target's pause hint as inline asm, as the C runtime's spin_relax, and
// nothing on a target without one. selfhost/llvm spin_hint_asm is the
// same table.
func TestSpinHintAsm(t *testing.T) {
	for _, c := range []struct{ arch, want string }{
		{"arm64", `call void asm sideeffect "yield", ""()`},
		{"amd64", `call void asm sideeffect "pause", ""()`},
		{"wasm", ""},
	} {
		if got := spinHintAsm(c.arch); got != c.want {
			t.Errorf("%s: %q, want %q", c.arch, got, c.want)
		}
	}
	if spinHintAsm("") == "" {
		t.Errorf("the host has no spin hint")
	}
}
