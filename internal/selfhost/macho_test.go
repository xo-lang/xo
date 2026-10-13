package selfhost

import (
	"testing"
)

// TestSelfhostMachoXoTests runs the Xo tests of selfhost/macho, the Mach-O
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
