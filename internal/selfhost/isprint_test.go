package selfhost

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var update = flag.Bool("update", false, "rewrite selfhost/lexer/isprint.xo")

// repoRoot is the repository root, two levels up from this package.
func repoRoot(t testing.TB) string {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// TestIsPrintTable keeps selfhost/lexer/isprint.xo in step with strconv.
func TestIsPrintTable(t *testing.T) {
	path := filepath.Join(repoRoot(t), "selfhost", "lexer", "isprint.xo")
	want := IsPrintTable()
	if *update {
		if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("%s is stale; run go test ./internal/selfhost -run TestIsPrintTable -update", path)
	}
}
