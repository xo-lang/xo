package selfhost

import (
	"testing"
)

// TestSelfhostMachoXoTests runs the Xo tests of compiler/macho, the Mach-O
// assembler and linker of decision 0182 (`xo test`): every instruction
// shape against the system assembler's encoding, logical immediates,
// integer parsing, compact unwind encodings, ULEB128 layout, errors. The
// comparison with `cc -c` on whole programs is scripts/macho-compare.py.
func TestSelfhostMachoXoTests(t *testing.T) {
	if selfhostTier(t) == tierShort {
		t.Skip("builds and runs the Xo test binary")
	}
	runXoTests(t, "macho")
}

// TestSelfhostArm64XoTests runs the Xo tests of compiler/arm64 (`xo
// test`): the constants mov_imm puts in one instruction.
func TestSelfhostArm64XoTests(t *testing.T) {
	if selfhostTier(t) == tierShort {
		t.Skip("builds and runs the Xo test binary")
	}
	runXoTests(t, "arm64")
}

// TestSelfhostDebugXoTests runs the Xo tests of compiler/debug (`xo
// test`), the DWARF reader of decision 0253: location expressions, a hand
// made DWARF 5 unit with a location list, the section readers.
func TestSelfhostDebugXoTests(t *testing.T) {
	if selfhostTier(t) == tierShort {
		t.Skip("builds and runs the Xo test binary")
	}
	runXoTests(t, "debug")
}
