package arm64

import (
	"bytes"
	"os"
	"os/exec"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// sepDarwinEnv is the environment of a darwin debug test's separately
// compiled build (decision 0388's default, with std's variants built
// first): os.Environ() without XO_SEPCOMP. Under the full tier
// (XO_TEST_SEPCOMP=0) every build is whole, so the case skips.
func sepDarwinEnv(t *testing.T) []string {
	t.Helper()
	if os.Getenv("XO_TEST_SEPCOMP") == "0" {
		t.Skip("XO_TEST_SEPCOMP=0: every build is whole program")
	}
	if err := xotool.StdVariants(); err != nil {
		t.Fatal(err)
	}
	return environWithout("XO_SEPCOMP")
}

// sepDsym checks that a build's --phase-times log says it linked std's
// store, then makes bin's dSYM with dsymutil from its debug map
// (decision 0401: the executable has __XODW, not __DWARF, and std's
// DWARF stays in the part objects N_OSO names), as a debugger would, and
// returns the dSYM's DWARF file.
func sepDsym(t *testing.T, bin string, log []byte) string {
	t.Helper()
	if !bytes.Contains(log, []byte("cache std ")) {
		t.Fatalf("the build was whole program, not separately compiled (std's variants: xotool.StdVariants):\n%s", lastLines(log, 20))
	}
	if hasDwarfSegment(t, bin) {
		t.Errorf("the separately compiled %s has a __DWARF segment (decision 0401: a debug map)", bin)
	}
	if b, err := testtmp.Guard(exec.Command("dsymutil", bin, "-o", bin+".dSYM")).CombinedOutput(); err != nil {
		t.Fatalf("dsymutil: %v\n%s", err, b)
	}
	return dwarfFile(bin)
}
