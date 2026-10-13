package mutlex

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/diag"
	golexer "github.com/xo-lang/xo/internal/lexer"
)

// TestSameAsLexer requires the copy to give internal/lexer's tokens
// (kind name, span, text) and error count on every .xo file of the
// repository. It goes with internal/lexer when the Go implementation is
// deleted (decision 0178 stage 7); the copy stays as it is.
func TestSameAsLexer(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && p != root && (strings.HasPrefix(d.Name(), ".") || d.Name() == "runs" || d.Name() == "output") {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(p, ".xo") {
			return nil
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		n++
		var ds diag.List
		want := golexer.Lex(p, src, &ds)
		var ms List
		got := Lex(p, src, &ms)
		if ms.N != len(ds.Items) {
			t.Errorf("%s: %d errors, internal/lexer %d", p, ms.N, len(ds.Items))
		}
		if len(got.Tokens) != len(want.Tokens) {
			t.Errorf("%s: %d tokens, internal/lexer %d", p, len(got.Tokens), len(want.Tokens))
			return nil
		}
		for i, w := range want.Tokens {
			g := got.Tokens[i]
			if g.Kind.String() != w.Kind.String() || g.Text != w.Text ||
				g.Span.Start.Offset != w.Span.Start.Offset || g.Span.End.Offset != w.Span.End.Offset {
				t.Errorf("%s: token %d is %s %q %d-%d, internal/lexer %s %q %d-%d", p, i,
					g.Kind, g.Text, g.Span.Start.Offset, g.Span.End.Offset, w.Kind, w.Text, w.Span.Start.Offset, w.Span.End.Offset)
				return nil
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n < 100 {
		t.Fatalf("lexed only %d files", n)
	}
	t.Logf("%d files", n)
}
