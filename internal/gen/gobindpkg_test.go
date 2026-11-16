package gen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

// TestGoBindOwnPackage builds the go_hex golden (`use go "encoding/hex"`)
// with the Go backend and checks that its adapter is written to package
// xoprog/gobind, not into the runtime: an adapter in rt made every
// program with a binding compile an rt of its own (30 rt variants on
// the 138 internal/gen goldens, 12 without the adapters; decision 0680).
// The golden's output checks that the program still builds and runs.
func TestGoBindOwnPackage(t *testing.T) {
	src := filepath.Join("testdata", "golden", "go_hex")
	dir := t.TempDir()
	tree := filepath.Join(dir, "gotree")
	res, err := xotool.Build([]string{src}, xotool.Options{Out: filepath.Join(dir, "prog"), Flags: []string{"--go-tree=" + tree}})
	if err != nil || res.Failed {
		t.Fatalf("the Go build failed: %v\n%s", err, diagText(res))
	}
	if m, _ := filepath.Glob(filepath.Join(tree, "rt", "xo_gobind_*.go")); len(m) > 0 {
		t.Errorf("adapters in the runtime package: %v", m)
	}
	// encoding/hex's adapter and those of the packages its API names
	// (fmt, io)
	m, _ := filepath.Glob(filepath.Join(tree, "gobind", "xo_gobind_*.go"))
	hex := false
	for _, f := range m {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), "\npackage gobind\n") {
			t.Errorf("%s is not in package gobind:\n%.300s", f, b)
		}
		hex = hex || filepath.Base(f) == "xo_gobind_encoding_hex.go"
	}
	if !hex {
		t.Fatalf("gobind/xo_gobind_*.go: %v, want encoding/hex's adapter", m)
	}
	rtFiles, _ := filepath.Glob(filepath.Join(tree, "rt", "*.go"))
	for _, f := range rtFiles {
		if b, err := os.ReadFile(f); err == nil && strings.Contains(string(b), "Go_encoding_hex_") {
			t.Errorf("%s names the binding's adapter", f)
		}
	}
}
