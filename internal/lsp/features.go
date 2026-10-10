package lsp

import (
	"sort"
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/format"
	"github.com/xo-lang/xo/internal/parser"
	"github.com/xo-lang/xo/internal/token"
)

// Semantic token legend. The order is the wire encoding; append only.
var tokenTypes = []string{
	"namespace", "type", "struct", "enum", "interface", "typeParameter",
	"parameter", "variable", "property", "enumMember", "function", "method",
}

var tokenModifiers = []string{"declaration", "readonly", "defaultLibrary"}

func (s *Server) format(p documentParams) (any, error) {
	path := uriToPath(p.TextDocument.URI)
	src, ok := s.docs[path]
	if !ok || s.isStdCopy(path) {
		return nil, nil
	}
	out, err := format.Source(path, src)
	if err != nil || string(out) == string(src) {
		// a file that does not parse is left alone; its errors are
		// already published
		return []textEdit{}, nil
	}
	t := newText(src)
	return []textEdit{{Range: lspRange{End: t.end()}, NewText: string(out)}}, nil
}

func (s *Server) symbols(p documentParams) (any, error) {
	path := uriToPath(p.TextDocument.URI)
	src, ok := s.docs[path]
	if !ok {
		return []documentSymbol{}, nil
	}
	f, _ := parser.ParseFile(path, src, 0)
	if f == nil {
		return []documentSymbol{}, nil
	}
	t := newText(src)
	rng := func(sp token.Span) lspRange {
		return lspRange{Start: t.offsetPosition(sp.Start.Offset), End: t.offsetPosition(sp.End.Offset)}
	}
	sym := func(name *ast.Ident, n ast.Node, kind int, detail string) documentSymbol {
		return documentSymbol{Name: name.Name, Detail: detail, Kind: kind, Range: rng(n.Span()), SelectionRange: rng(name.Sp)}
	}
	out := []documentSymbol{}
	for _, d := range f.Decls {
		switch x := d.(type) {
		case *ast.FnDecl:
			if x.Name == nil || x.Name.Name == "" {
				continue
			}
			ds := sym(x.Name, x, symFunction, "")
			if x.Recv != nil {
				ds.Kind = symMethod
				if nt, ok := x.Recv.Type.(*ast.NamedType); ok && len(nt.Path) > 0 {
					ds.Name = nt.Path[len(nt.Path)-1].Name + "." + ds.Name
				}
			}
			out = append(out, ds)
		case *ast.StructDecl:
			ds := sym(x.Name, x, symStruct, "")
			for _, fl := range x.Fields {
				if fl.Name != nil {
					ds.Children = append(ds.Children, sym(fl.Name, fl, symProperty, ""))
				}
			}
			out = append(out, ds)
		case *ast.EnumDecl:
			ds := sym(x.Name, x, symEnum, "")
			for _, v := range x.Variants {
				if v.Name != nil {
					ds.Children = append(ds.Children, sym(v.Name, v, symEnumMember, ""))
				}
			}
			out = append(out, ds)
		case *ast.InterfaceDecl:
			ds := sym(x.Name, x, symInterface, "")
			for _, m := range x.Methods {
				if m.Name != nil {
					ds.Children = append(ds.Children, sym(m.Name, m, symMethod, ""))
				}
			}
			out = append(out, ds)
		case *ast.TypeDecl:
			detail := "alias"
			if x.Newtype {
				detail = "newtype"
			}
			out = append(out, sym(x.Name, x, symClass, detail))
		case *ast.ConstDecl:
			out = append(out, sym(x.Name, x, symConstant, ""))
		case *ast.TestDecl:
			if x.Name == nil {
				continue
			}
			name := strings.Trim(string(src[x.Name.Sp.Start.Offset:x.Name.Sp.End.Offset]), `"`)
			out = append(out, documentSymbol{Name: "test " + name, Kind: symFunction, Range: rng(x.Sp), SelectionRange: rng(x.Name.Sp)})
		}
	}
	return out, nil
}

func (s *Server) semanticTokens(p documentParams) (any, error) {
	snap := s.fresh()
	path := uriToPath(p.TextDocument.URI)
	f, t := snap.file(path)
	if f == nil || snap.info == nil {
		return map[string]any{"data": []int{}}, nil
	}
	typeIndex := map[string]int{}
	for i, n := range tokenTypes {
		typeIndex[n] = i
	}
	type tok struct{ line, char, length, typ, mods int }
	var toks []tok
	seen := map[int]bool{}
	for _, site := range snap.identsOf(f) {
		id := site.id
		if id.Name == "" || seen[id.Sp.Start.Offset] || id.Sp.End.Offset <= id.Sp.Start.Offset {
			continue
		}
		// identifiers the parser synthesized (such as `self` in a bare
		// receiver) do not match the source text
		if string(f.Src[id.Sp.Start.Offset:id.Sp.End.Offset]) != id.Name {
			continue
		}
		tg := snap.resolve(f, site)
		ti, ok := typeIndex[tg.kind]
		if !ok {
			continue
		}
		seen[id.Sp.Start.Offset] = true
		start := t.offsetPosition(id.Sp.Start.Offset)
		toks = append(toks, tok{start.Line, start.Character, utf16Len([]byte(id.Name)), ti, tg.mods})
	}
	sort.Slice(toks, func(i, j int) bool {
		return toks[i].line < toks[j].line || toks[i].line == toks[j].line && toks[i].char < toks[j].char
	})
	data := make([]int, 0, 5*len(toks))
	pl, pc := 0, 0
	for _, k := range toks {
		dl := k.line - pl
		dc := k.char
		if dl == 0 {
			dc = k.char - pc
		}
		data = append(data, dl, dc, k.length, k.typ, k.mods)
		pl, pc = k.line, k.char
	}
	return map[string]any{"data": data}, nil
}
