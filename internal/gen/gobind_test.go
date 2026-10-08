package gen

import (
	"bytes"
	"fmt"
	goast "go/ast"
	goparser "go/parser"
	"go/printer"
	gotoken "go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/check"
)

// TestGoBindingSignatures binds a set of Go standard library packages (Go
// interop, decision 0041) and checks that every bound function, static,
// and method has an adapter whose Go signature is exactly what the code
// generator calls: the parameter and result types gen derives from the Xo
// declaration (pkgGen.goType, goResult) against the adapter's go/ast.
func TestGoBindingSignatures(t *testing.T) {
	if testing.Short() {
		t.Skip("binds many packages")
	}
	dir := t.TempDir()
	t.Setenv("XO_GOBIND_DIR", filepath.Join(dir, "gobind"))
	pkgs := []string{"crypto/sha256", "encoding/hex", "regexp", "strings", "strconv", "net/url",
		"bytes", "time", "math/big", "bufio", "io", "os", "os/exec", "net/http", "log/slog",
		"database/sql", "encoding/json", "path/filepath", "unicode/utf8", "archive/zip"}
	var src strings.Builder
	for i, p := range pkgs {
		fmt.Fprintf(&src, "use go %q as p%d\n", p, i)
	}
	src.WriteString("\nfn main() {}\n")
	mdir := filepath.Join(dir, "m")
	os.MkdirAll(mdir, 0o755)
	if err := os.WriteFile(filepath.Join(mdir, "main.xo"), []byte(src.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := check.Check([]string{mdir})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range res.Diagnostics {
		if d.Code != "XO0110" {
			t.Errorf("unexpected diagnostic %s %s at %s:%d", d.Code, d.Message, d.Span.File, d.Span.Line)
		}
	}
	info := res.Info()
	g := &gen{info: info, pkgs: map[*check.Module]*pkgGen{}, modRoot: map[string]string{}, prelude: info.Prelude()}
	pg := &pkgGen{g: g, name: "rt", imports: map[string]string{}, adapt: map[string]bool{}, inRT: true}
	norm := func(s string) string { return strings.ReplaceAll(s, " ", "") }
	total := 0
	for _, m := range info.AllModules() {
		if m.Go == nil {
			continue
		}
		fset := gotoken.NewFileSet()
		af, err := goparser.ParseFile(fset, m.Go.GoPath+".go", m.Go.Adapter, 0)
		if err != nil {
			t.Fatalf("%s: adapter does not parse: %v", m.Go.GoPath, err)
		}
		decls := map[string]*goast.FuncDecl{}
		for _, d := range af.Decls {
			fd, ok := d.(*goast.FuncDecl)
			if !ok {
				continue
			}
			key := fd.Name.Name
			if fd.Recv != nil {
				key = exprString(fset, fd.Recv.List[0].Type) + "." + key
			}
			decls[key] = fd
		}
		var fos []*check.FuncObj
		for _, o := range m.Scope {
			switch x := o.(type) {
			case *check.FuncObj:
				fos = append(fos, x)
			case *check.TypeObj:
				for _, mo := range x.Methods {
					fos = append(fos, mo)
				}
			}
		}
		sort.Slice(fos, func(i, j int) bool { return fos[i].DisplayName() < fos[j].DisplayName() })
		for _, fo := range fos {
			var key string
			switch {
			case fo.Recv == nil:
				key = m.Go.Prefix + fo.Name
			case fo.Static:
				key = m.Go.Prefix + fo.Recv.Name + "_" + fo.Name
			default:
				key = m.Go.Prefix + fo.Recv.Name + ".M_" + fo.Name
			}
			fd := decls[key]
			if fd == nil {
				t.Errorf("%s: no adapter %s for %s", m.Go.GoPath, key, fo.DisplayName())
				continue
			}
			total++
			want := []string{"*Task"}
			for _, p := range fo.Params {
				want = append(want, pg.goType(p.Type))
			}
			var got []string
			for _, f := range fd.Type.Params.List {
				ts := exprString(fset, f.Type)
				n := len(f.Names)
				if n == 0 {
					n = 1
				}
				for i := 0; i < n; i++ {
					got = append(got, ts)
				}
			}
			wantRes := pg.goResult(fo)
			gotRes := ""
			if fd.Type.Results != nil {
				gotRes = exprString(fset, fd.Type.Results.List[0].Type)
			}
			if norm(strings.Join(got, ",")) != norm(strings.Join(want, ",")) || norm(gotRes) != norm(wantRes) {
				t.Errorf("%s: %s: adapter (%s) %s, gen calls (%s) %s", m.Go.GoPath, fo.DisplayName(), strings.Join(got, ", "), gotRes, strings.Join(want, ", "), wantRes)
			}
		}
	}
	if total < 1000 {
		t.Errorf("only %d bound declarations checked", total)
	}
	t.Logf("%d bound declarations match their adapters", total)
}

func exprString(fset *gotoken.FileSet, e goast.Expr) string {
	var b bytes.Buffer
	printer.Fprint(&b, fset, e)
	return b.String()
}
