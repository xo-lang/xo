package gen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

// TestGoProtoNotKept builds the go_proto_roots golden (internal/llvm)
// with the Go backend, specialized and with --no-mono, and checks that
// Unused, which is never compared or hashed, has no XoEq or XoHash
// (decision 0542). The golden's output checks that every compared type
// keeps them; this checks the other side, which no output shows: a
// generic form that runs under --no-mono (or past the copy cap) once set
// every type's methods (armnetwork kept all 3,290 types, 2026-11-07).
func TestGoProtoNotKept(t *testing.T) {
	src := filepath.Join("..", "llvm", "testdata", "golden", "go_proto_roots")
	for _, noMono := range []bool{false, true} {
		noMono := noMono
		name := "mono"
		if noMono {
			name = "nomono"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			tree := filepath.Join(dir, "gotree")
			res, err := xotool.Build([]string{src}, xotool.Options{NoMono: noMono, Out: filepath.Join(dir, "prog"), Flags: []string{"--go-tree=" + tree}})
			if err != nil || res.Failed {
				t.Fatalf("the Go build failed: %v\n%s", err, diagText(res))
			}
			var text strings.Builder
			err = filepath.Walk(tree, func(p string, info os.FileInfo, err error) error {
				if err != nil || info.IsDir() || !strings.HasSuffix(p, ".go") {
					return err
				}
				b, err := os.ReadFile(p)
				text.Write(b)
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
			got := text.String()
			if !strings.Contains(got, "func (v Sent) XoEq(") {
				t.Errorf("Sent, passed to a constrained std function, has no XoEq")
			}
			for _, m := range []string{"XoEq(", "XoEqT(", "XoHash("} {
				if strings.Contains(got, "func (v Unused) "+m) {
					t.Errorf("Unused, never compared or hashed, has %s)", m)
				}
			}
		})
	}
}
