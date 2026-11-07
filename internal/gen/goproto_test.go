package gen

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

// TestGoProtoNotKept builds the go_proto_roots golden (internal/llvm)
// with the Go backend, specialized and with --no-mono, and checks that
// Unused, which is never compared or hashed, has no XoEq or XoHash
// (decision 0542), nor Sent, which only json.encode_str sees (listed in
// std/go_proto_free.txt). The golden's output checks that every compared type
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
			if !strings.Contains(got, "func (v Pair) XoEq(") {
				t.Errorf("Pair, compared by a generic function, has no XoEq")
			}
			for _, typ := range []string{"Unused", "Sent"} {
				for _, m := range []string{"XoEq(", "XoEqT(", "XoHash("} {
					if strings.Contains(got, "func (v "+typ+") "+m) {
						t.Errorf("%s, never compared or hashed, has %s)", typ, m)
					}
				}
			}
		})
	}
}

// TestGoProtoFreeListed checks that each line of std/go_proto_free.txt
// names a generic function of std/rt's hand written Go (`json.encode` is
// Json_encode[T any]): a line that names nothing would silently stop
// rooting another function's type arguments after a rename.
func TestGoProtoFreeListed(t *testing.T) {
	root := filepath.Join("..", "..")
	b, err := os.ReadFile(filepath.Join(root, "std", "go_proto_free.txt"))
	if err != nil {
		t.Fatal(err)
	}
	generic := map[string]bool{}
	fset := token.NewFileSet()
	files, _ := filepath.Glob(filepath.Join(root, "std", "rt", "*.go"))
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		af, err := parser.ParseFile(fset, f, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range af.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok && fd.Recv == nil && fd.Type.TypeParams != nil {
				generic[fd.Name.Name] = true
			}
		}
	}
	n := 0
	for _, l := range strings.Split(string(b), "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		n++
		mod, name, ok := strings.Cut(l, ".")
		if !ok {
			t.Errorf("%q: want module.name", l)
			continue
		}
		goName := strings.ToUpper(mod[:1]) + mod[1:] + "_" + name
		if !generic[goName] {
			t.Errorf("%q: std/rt has no generic function %s", l, goName)
		}
	}
	if n == 0 {
		t.Error("std/go_proto_free.txt lists nothing")
	}
}
