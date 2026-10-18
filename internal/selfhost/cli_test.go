package selfhost

import (
	"testing"
)

// TestSelfhostCLIXoTests runs the Xo tests of compiler/cli/dev (the
// pattern, expectation, and output helpers of xo dev codegen, errorcheck,
// compare, goldens, and key) and compiler/cli/cmdhelp (every command has
// a usage; package patterns), moved out of the sources into _test.xo
// files by decision 0262.
func TestSelfhostCLIXoTests(t *testing.T) {
	if selfhostTier(t) == tierShort {
		t.Skip("builds and runs the Xo test binary")
	}
	for _, c := range []string{"cli/dev", "cli/cmdhelp"} {
		t.Run(c, func(t *testing.T) { runXoTests(t, c) })
	}
}
