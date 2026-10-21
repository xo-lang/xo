package repotest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xo-lang/xo/internal/stdindex"
)

// TestStdIndexCurrent keeps spec/std.md's prelude index (Appendix A)
// equal to what std/prelude.xo declares: run `go run ./scripts/stdindex`
// after changing the prelude (2026-10-20 poll: every prelude method
// listed with its signature).
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
		t.Errorf("spec/std.md's prelude index is not std/prelude.xo's: run `go run ./scripts/stdindex`")
	}
}
