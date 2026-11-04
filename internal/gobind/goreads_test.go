package gobind

import (
	"go/importer"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestGoReadsList reviews compiler/check/goreads.xo, the Go functions
// XO1107 trusts to only read the List, Map or Set given as `any`
// (decision 0491 B): the entries are in order and unique, and each
// standard library entry names a Go function or method, by the binding's
// Xo name, with an interface parameter (a stale or misspelled entry
// would silence nothing, or the wrong call).
func TestGoReadsList(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "compiler", "check", "goreads.xo"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	i := strings.Index(body, "key is ")
	if i < 0 {
		t.Fatal("goreads.xo has no `key is` list")
	}
	var ents []string
	for _, m := range regexp.MustCompile(`"([^"]+)"`).FindAllStringSubmatch(body[i:], -1) {
		ents = append(ents, m[1])
	}
	if len(ents) < 10 {
		t.Fatalf("only %d entries", len(ents))
	}
	if !sort.StringsAreSorted(ents) {
		t.Errorf("entries out of order")
	}
	for j := 1; j < len(ents); j++ {
		if ents[j] == ents[j-1] {
			t.Errorf("duplicate entry %s", ents[j])
		}
	}
	imp := importer.ForCompiler(token.NewFileSet(), "source", nil)
	for _, e := range ents {
		slash := strings.LastIndex(e, "/")
		dot := strings.Index(e[slash+1:], ".")
		if dot < 0 {
			t.Errorf("%s: no name after the package", e)
			continue
		}
		path, name := e[:slash+1+dot], e[slash+1+dot+1:]
		if strings.Contains(strings.SplitN(path, "/", 2)[0], ".") {
			continue // a module outside the standard library: named only
		}
		pkg, err := imp.Import(path)
		if err != nil {
			t.Errorf("%s: %v", e, err)
			continue
		}
		if fn := lookupXoName(pkg, name); fn == nil {
			t.Errorf("%s: no Go function or method of %s binds as %s", e, path, name)
		} else if !hasIfaceParam(fn.Type().(*types.Signature)) {
			t.Errorf("%s: %s has no interface parameter", e, fn.FullName())
		}
	}
}

func lookupXoName(pkg *types.Package, name string) *types.Func {
	tn, mn, isMethod := strings.Cut(name, ".")
	for _, n := range pkg.Scope().Names() {
		o := pkg.Scope().Lookup(n)
		if !o.Exported() {
			continue
		}
		if !isMethod {
			if f, ok := o.(*types.Func); ok && xoName(snake(n)) == name {
				return f
			}
			continue
		}
		if n != tn {
			continue
		}
		ms := types.NewMethodSet(types.NewPointer(o.Type()))
		for k := 0; k < ms.Len(); k++ {
			if f, ok := ms.At(k).Obj().(*types.Func); ok && f.Exported() && xoName(snake(f.Name())) == mn {
				return f
			}
		}
	}
	return nil
}

func hasIfaceParam(sig *types.Signature) bool {
	for k := 0; k < sig.Params().Len(); k++ {
		t := sig.Params().At(k).Type()
		if s, ok := t.(*types.Slice); ok && sig.Variadic() && k == sig.Params().Len()-1 {
			t = s.Elem()
		}
		if types.IsInterface(t) {
			return true
		}
	}
	return false
}
