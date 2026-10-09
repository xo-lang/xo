package parser

import (
	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/token"
)

// Type habits (decision 0087): `Vec<T>`, `[]T`, `map[K]V`, `*T`, `&T`,
// `?T`, `interface{}`.

// angleState collects the edits of one outermost `<...>` type argument
// list so nested lists produce a single diagnostic.
type angleState struct {
	edits     []diag.Edit
	pendingGt bool // an inner list consumed `>>`, closing this one too
}

// angleArgs parses `Name<A, B>` type arguments.
func (p *parser) angleArgs(nt *ast.NamedType) {
	outer := p.angle == nil
	if outer {
		p.angle = &angleState{}
	}
	st := p.angle
	open := p.tok
	p.next() // <
	st.edits = append(st.edits, p.replaceSpan(open.Span, "["))
	for !p.bad {
		nt.Args = append(nt.Args, p.parseType())
		if p.bad || st.pendingGt {
			break
		}
		if !p.got(token.Comma) {
			break
		}
	}
	end := p.tok.Span.End
	switch {
	case p.bad:
	case st.pendingGt:
		st.pendingGt = false
		end = p.tokAt(p.pos - 1).Span.End
	case p.at(token.Gt):
		st.edits = append(st.edits, p.replaceSpan(p.tok.Span, "]"))
		p.next()
	case p.at(token.Shr) && !outer:
		st.edits = append(st.edits, p.replaceSpan(p.tok.Span, "]]"))
		st.pendingGt = true
		p.next()
	default:
		p.errorExpected("`>` to close the type arguments")
	}
	if !outer {
		return
	}
	p.angle = nil
	name := p.spanText(nt.Path[len(nt.Path)-1].Sp)
	d := p.foreign(token.Span{File: p.file, Start: open.Span.Start, End: end}, "`%s<...>` is Rust, TypeScript, Kotlin, and Swift syntax for type arguments; Xo writes them in square brackets, `%s[...]` (core 2.7)", name, name)
	if !p.bad {
		addFix(d, "use square brackets", st.edits...)
	}
}

// angleTypeParams parses the type parameters of a declaration written in
// angle brackets, `fn f<T: Ord>(...)`, with elem parsing one parameter.
func (p *parser) angleTypeParams(elem func()) {
	open := p.tok
	p.next() // <
	for !p.bad && !p.at(token.Gt) && !p.at(token.EOF) {
		elem()
		if !p.got(token.Comma) {
			break
		}
	}
	closeTok := p.tok
	if !p.at(token.Gt) {
		p.errorExpected("`>` to close the type parameters")
		return
	}
	p.next()
	d := p.foreign(open.Span.To(closeTok.Span), "`<T>` type parameters are Rust, TypeScript, Kotlin, and Swift syntax; Xo writes them in square brackets, `[T]` (core 2.7)")
	addFix(d, "use square brackets", p.replaceSpan(open.Span, "["), p.replaceSpan(closeTok.Span, "]"))
}

// swiftCollectionType parses Swift's `[T]` (an array) and `[K: V]` (a
// dictionary) at `[`.
func (p *parser) swiftCollectionType() (ast.Type, bool) {
	if !p.startsTypeAt(p.pos + 1) {
		return nil, false
	}
	open := p.tok
	start := open.Span.Start
	p.next() // [
	elem := p.parseType()
	if p.at(token.Colon) {
		colon := p.tok
		p.next()
		val := p.parseType()
		closeTok := p.tok
		p.expect(token.RBrack, "`]`")
		d := p.foreign(open.Span.To(closeTok.Span), "`[K: V]` is Swift's dictionary type; Xo writes `Map[K, V]` (core 2.1)")
		addFix(d, "write `Map[K, V]`", p.insertAt(open.Span.Start, "Map"), p.replaceSpan(colon.Span, ","))
		return &ast.NamedType{Base: ast.Base{Sp: p.spanFrom(start)}, Path: []*ast.Ident{ident("Map", open.Span)}, Args: []ast.Type{elem, val}}, true
	}
	closeTok := p.tok
	p.expect(token.RBrack, "`]`")
	d := p.foreign(open.Span.To(closeTok.Span), "`[T]` is Swift's array type; Xo writes `List[T]` (core 2.1)")
	addFix(d, "write `List[T]`", p.insertAt(open.Span.Start, "List"))
	return &ast.NamedType{Base: ast.Base{Sp: p.spanFrom(start)}, Path: []*ast.Ident{ident("List", open.Span)}, Args: []ast.Type{elem}}, true
}

// foreignType recognizes types written with another language's syntax at
// the start of a type. ok is false when none applies.
func (p *parser) foreignType() (ast.Type, bool) {
	t := p.tok
	start := t.Span.Start
	switch t.Kind {
	case token.Star, token.Amp, token.AndAnd:
		// Go `*T`, Rust `&T`, `&mut T`, `&'a T` (the lifetime lexes as a
		// rune error, so only the plain forms are handled).
		end := p.pos + 1
		if p.tokAt(end).Kind == token.Ident && p.tokAt(end).Text == "mut" && p.tokAt(end+1).Kind != token.Comma && p.tokAt(end+1).Kind != token.RParen {
			end++
		}
		if !p.startsTypeAt(end) {
			return nil, false
		}
		what, lang := "a pointer type", "Go"
		if t.Kind != token.Star {
			what, lang = "a reference type", "Rust"
		}
		d := p.foreign(t.Span, "`%s` is %s, %s syntax; Xo has no pointers or references: values are passed and stored as is, and a `var` parameter changes the caller's binding (core 5.1, 3.2)", p.srcText(t.Span.Start, p.tokAt(end).Span.Start), what, lang)
		addFix(d, "remove `"+p.srcText(t.Span.Start, p.tokAt(end).Span.Start)+"`", p.replaceRange(t.Span.Start, p.tokAt(end).Span.Start, ""))
		p.skipTo(end)
		return p.parseType(), true
	case token.Question:
		if !p.startsTypeAt(p.pos + 1) {
			return nil, false
		}
		p.next()
		inner := p.parseType()
		d := p.foreign(t.Span, "`?T` puts the option mark first; Xo writes it after the type, `%s?` (core 2.5)", p.spanText(inner.Span()))
		addFix(d, "move `?` after the type", p.replaceSpan(t.Span, ""), p.insertAt(inner.Span().End, "?"))
		return &ast.OptionalType{Base: ast.Base{Sp: p.spanFrom(start)}, Elem: inner}, true
	case token.LBrack:
		if p.peek(1).Kind != token.RBrack {
			return p.swiftCollectionType()
		}
		// Go slices: `[]T`.
		closeTok := p.peek(1)
		p.next()
		p.next()
		elem := p.parseType()
		d := p.foreign(t.Span.To(closeTok.Span), "`[]%s` is Go syntax for a slice; Xo writes `List[%s]` (core 2.1)", p.spanText(elem.Span()), p.spanText(elem.Span()))
		addFix(d, "write `List[...]`", p.replaceSpan(t.Span.To(closeTok.Span), "List["), p.insertAt(elem.Span().End, "]"))
		sp := p.spanFrom(start)
		return &ast.NamedType{Base: ast.Base{Sp: sp}, Path: []*ast.Ident{ident("List", t.Span)}, Args: []ast.Type{elem}}, true
	case token.Ident:
		if t.Text != "map" || p.peek(1).Kind != token.LBrack || !adjacent(t, p.peek(1)) {
			return nil, false
		}
		// Go maps: `map[K]V`.
		p.next()
		lb := p.tok
		p.next()
		key := p.parseType()
		rb := p.tok
		if !p.at(token.RBrack) || !p.startsTypeAt(p.pos+1) {
			p.errorExpected("`]` after the map key type")
			return &ast.BadType{Base: ast.Base{Sp: t.Span}}, true
		}
		p.next()
		val := p.parseType()
		d := p.foreign(t.Span.To(val.Span()), "`map[%s]%s` is Go syntax; Xo writes `Map[%s, %s]` (core 2.1)", p.spanText(key.Span()), p.spanText(val.Span()), p.spanText(key.Span()), p.spanText(val.Span()))
		addFix(d, "write `Map[K, V]`", p.replaceRange(t.Span.Start, lb.Span.End, "Map["), p.replaceRange(rb.Span.Start, val.Span().Start, ", "), p.insertAt(val.Span().End, "]"))
		sp := p.spanFrom(start)
		return &ast.NamedType{Base: ast.Base{Sp: sp}, Path: []*ast.Ident{ident("Map", t.Span)}, Args: []ast.Type{key, val}}, true
	case token.Interface:
		if p.peek(1).Kind != token.LBrace || p.peek(2).Kind != token.RBrace {
			return nil, false
		}
		end := p.peek(2)
		p.foreign(t.Span.To(end.Span), "`interface{}` is Go's empty interface; Xo has no top type: make the function generic, `fn f[T](x: T)`, or name an interface with the methods you need (core 2.4, 2.7)")
		p.next()
		p.next()
		p.next()
		return &ast.BadType{Base: ast.Base{Sp: t.Span.To(end.Span)}}, true
	}
	return nil, false
}
