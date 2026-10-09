package gen

import (
	"bytes"
	"fmt"
	"go/format"
	"regexp"
	"sort"
	"strings"

	"github.com/xo-lang/xo/internal/check"
)

// Hybrid builds (`xo build --hybrid`, decision 0084): hot leaf functions
// of the main module run as Go assembler that internal/arm64 emits (Go
// mode), the rest of the program is the Go backend's. For each such
// function the package gets
//
//   - xo_hybrid.go (arm64, not tag xo_pure): the function as a wrapper
//     that makes each `var` List it writes unique (as an element write
//     does) and calls the assembly function with the Lists' slices, the
//     Go declarations of the assembly functions, and xoHybridFault, which
//     the assembly calls to fault (rt.HybridFault);
//   - xo_pure.go (other architectures, or tag xo_pure): the function as
//     the Go backend writes it, the fallback and the reference of the
//     differential tests (`go build -tags xo_pure`);
//   - xo_hybrid_arm64.s: the assembly.

// Hybrid is the assembly of a hybrid build and what the Go side needs.
type Hybrid struct {
	// Funcs are the assembly functions, by Xo name (functions of the main
	// module).
	Funcs map[string]HybridFunc
	// Positions are the fault positions xoHybridFault's pos indexes.
	Positions []string
	// Asm is the .s file.
	Asm []byte
}

// HybridFunc is one assembly function.
type HybridFunc struct {
	Sym string
	// Wrap: the Go function calls the assembly one (otherwise only other
	// assembly functions do).
	Wrap     bool
	NoEscape bool
	// Unshare are the indexes of the `var` List parameters the function
	// writes.
	Unshare []int
}

// hybridPkg is the hybrid output of the main module's package.
type hybridPkg struct {
	pure  bytes.Buffer // the Go functions, for xo_pure.go
	wraps bytes.Buffer // wrappers and declarations, for xo_hybrid.go
}

// hybridFunc reports whether fo is an assembly function of a hybrid
// build.
func (pg *pkgGen) hybridFunc(fo *check.FuncObj) (HybridFunc, bool) {
	h := pg.g.opts.Hybrid
	if h == nil || pg.g.opts.Mode != Build || fo.Recv != nil || len(typeParams(fo.TParams)) > 0 || pg.m == nil || !pg.m.Root {
		return HybridFunc{}, false
	}
	if _, ok := pg.m.Scope["main"].(*check.FuncObj); !ok {
		return HybridFunc{}, false
	}
	hf, ok := h.Funcs[fo.Name]
	return hf, ok
}

// hybridSlice is the Go element type of a List type ("" for another
// type).
func (pg *pkgGen) hybridSlice(t check.Type) string {
	if !pg.g.isPrelude(t, "List") {
		return ""
	}
	return pg.goType(typeArgs(pg.g.named(t))[0])
}

// genHybrid writes the declaration of fo's assembly function and, when
// it is wrapped, the wrapper (name and params as genFuncWith declares
// them: "v_xs *rt.List[Rec]"); it returns false when the Go side cannot
// take it (the function stays Go only).
func (pg *pkgGen) genHybrid(fo *check.FuncObj, hf HybridFunc, name string, pnames []string, sig string) {
	h := pg.hyb
	if h == nil {
		h = &hybridPkg{}
		pg.hyb = h
	}
	var decl, call []string
	unshare := map[int]bool{}
	for _, i := range hf.Unshare {
		unshare[i] = true
	}
	var pre []string
	for i, p := range fo.Params {
		ref := pnames[i]
		if p.Var {
			ref = "(*" + ref + ")"
		}
		if et := pg.hybridSlice(p.Type); et != "" {
			decl = append(decl, fmt.Sprintf("p%d []%s", i, et))
			if unshare[i] {
				pre = append(pre, fmt.Sprintf("\tif !%sList_Mine(%s) {\n\t\t%s = %sList_Unshared(%s)\n\t}\n", pg.rt(), ref, ref, pg.rt(), ref))
			}
			call = append(call, pg.rt()+"List_Slice("+ref+")")
			continue
		}
		decl = append(decl, fmt.Sprintf("p%d %s", i, pg.goType(p.Type)))
		call = append(call, ref)
	}
	res := ""
	if !returnsVoid(fo) {
		res = " (r0 " + pg.goType(fo.Result) + ")"
	}
	w := &h.wraps
	if hf.NoEscape {
		w.WriteString("//go:noescape\n")
	}
	fmt.Fprintf(w, "func %s(%s)%s\n\n", hf.Sym, strings.Join(decl, ", "), res)
	if !hf.Wrap {
		return
	}
	fmt.Fprintf(w, "%s {\n", sig)
	for _, p := range pre {
		w.WriteString(p)
	}
	ret := ""
	if res != "" {
		ret = "return "
	}
	fmt.Fprintf(w, "\t%s%s(%s)\n}\n\n", ret, hf.Sym, strings.Join(call, ", "))
}

// hybridFiles returns the files of a hybrid package: xo.go (body is the
// package's code without the wrapped functions), xo_pure.go,
// xo_hybrid.go, and the assembly; imports a file does not use are left
// out of it.
func (pg *pkgGen) hybridFiles(body []byte) (map[string][]byte, error) {
	h := pg.g.opts.Hybrid
	var fault bytes.Buffer
	fault.WriteString("// xoHybridFault raises the fault of an assembly function (rt.HybridFault).\n")
	fault.WriteString("func xoHybridFault(kind, a, b, c, pos int64) {\n\t" + pg.rt() + "HybridFault(kind, a, b, c, xoHybridPos[pos])\n}\n\n")
	fault.WriteString("var xoHybridPos = [...]string{")
	for i, p := range h.Positions {
		if i > 0 {
			fault.WriteString(", ")
		}
		fmt.Fprintf(&fault, "%q", p)
	}
	fault.WriteString("}\n")
	files := map[string][]byte{}
	for _, f := range []struct {
		name, tag string
		body      []byte
	}{
		{"xo.go", "", body},
		{"xo_pure.go", "!arm64 || xo_pure", pg.hyb.pure.Bytes()},
		{"xo_hybrid.go", "arm64 && !xo_pure", append(append([]byte(nil), pg.hyb.wraps.Bytes()...), fault.Bytes()...)},
	} {
		src, err := pg.fileWith(f.tag, f.body, true)
		if err != nil {
			return nil, err
		}
		files[pg.dir+"/"+f.name] = src
	}
	files[pg.dir+"/xo_hybrid_arm64.s"] = h.Asm
	return files, nil
}

// fileWith is file with a build constraint and a body; filter leaves out
// the imports the body does not name.
func (pg *pkgGen) fileWith(tag string, body []byte, filter bool) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString("// Code generated by xo from module " + pg.m.Path + ". DO NOT EDIT.\n\n")
	if tag != "" {
		b.WriteString("//go:build " + tag + "\n\n")
	}
	b.WriteString("package " + pg.name + "\n\n")
	var paths []string
	for p, name := range pg.imports {
		if filter && !usesIdent(body, name) {
			continue
		}
		paths = append(paths, p)
	}
	sort.Strings(paths)
	if len(paths) > 0 {
		b.WriteString("import (\n")
		for _, p := range paths {
			fmt.Fprintf(&b, "\t%s %q\n", pg.imports[p], p)
		}
		b.WriteString(")\n\n")
	}
	b.Write(body)
	src, err := format.Source(b.Bytes())
	if err != nil {
		return b.Bytes(), fmt.Errorf("gen: generated Go for module %s does not parse: %v", pg.m.Path, err)
	}
	return src, nil
}

// usesIdent reports whether src names the package name (`name.`).
func usesIdent(src []byte, name string) bool {
	re := regexp.MustCompile(`(^|[^A-Za-z0-9_.])` + regexp.QuoteMeta(name) + `\.`)
	return re.Match(src)
}
