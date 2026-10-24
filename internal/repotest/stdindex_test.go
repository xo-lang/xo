package repotest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xo-lang/xo/internal/stdindex"
	"github.com/xo-lang/xo/internal/stdsync"
)

// TestStdIndexCurrent keeps spec/std.md's prelude index (Appendix A)
// equal to what std/prelude.xo declares: run `go run ./scripts/stdsync`
// after changing the prelude (it also regenerates the other files a
// prelude change derives; 2026-10-20 poll: every prelude method listed
// with its signature).
func TestStdIndexCurrent(t *testing.T) {
	prelude, err := os.ReadFile(filepath.Join(root, "std", "prelude.xo"))
	if err != nil {
		t.Fatal(err)
	}
	spec, err := os.ReadFile(filepath.Join(root, "spec", "std.md"))
	if err != nil {
		t.Fatal(err)
	}
	want := stdindex.Index(string(prelude))
	if got := stdindex.Current(string(spec)); got != want {
		t.Errorf("spec/std.md's prelude index is not std/prelude.xo's: run `%s`", stdsync.Command)
	}
}
