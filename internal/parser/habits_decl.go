package parser

import (
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/token"
)

// Top level and declaration habits (decision 0087).

// fnKeywords maps a foreign function keyword to the languages it comes from.
var fnKeywords = map[string]string{
	"func":     "Go and Swift",
	"fun":      "Kotlin",
	"def":      "Python",
	"function": "TypeScript",
}

// moveUseFix gives XO0173 a fix that moves the misplaced `use` line up to
// the other imports (or before the first declaration).
func (p *parser) moveUseFix(d *diag.Diagnostic, f *ast.File, u *ast.UseDecl) {
	if d == nil {
		return
	}
	addNote(d, "Python and TypeScript allow imports anywhere; an Xo file lists every `use` first (core 15: File)")
	ls := p.lineStart(u.Sp.Start)
	if strings.TrimSpace(p.srcText(ls, u.Sp.Start)) != "" {
		return
	}
	at := p.firstDecl
	for _, prev := range f.Uses {
		if prev.Sp.End.Offset < p.firstDecl.Offset {
			at = p.nextLineStart(prev.Sp.End)
		}
	}
	at = p.lineStart(at)
	line := strings.TrimSpace(p.srcText(u.Sp.Start, u.Sp.End)) + "\n"
	if at.Offset == p.lineStart(p.firstDecl).Offset {
		line += "\n" // a blank line between the imports and the first declaration
	}
	addFix(d, "move the `use` above the declarations",
		p.insertAt(at, line),
		p.replaceRange(ls, p.nextLineStart(u.Sp.End), ""))
}

// foreignTopLevel recognizes declarations written in another language at
// top level. ok is false when the token is not a known habit.
func (p *parser) foreignTopLevel(f *ast.File, doc *ast.Doc, pub bool, pubTok token.Token, start token.Pos, seenDecl bool) (ast.Decl, bool) {
	t := p.tok
	if t.Kind != token.Ident {
		if t.Kind == token.LParen && pub && p.peek(1).Kind == token.Ident && p.peek(2).Kind == token.RParen {
			switch p.peek(1).Text {
			case "crate", "super", "self":
				// pub(crate): Rust visibility scopes.
				end := p.peek(2)
				d := p.foreign(t.Span.To(end.Span), "`pub(%s)` is Rust syntax; Xo has one visibility: `pub` exports from the module (core 1.5)", p.peek(1).Text)
				addFix(d, "write `pub`", p.replaceSpan(t.Span.To(end.Span), ""))
				p.next()
				p.next()
				p.next()
				return p.parseTopDecl(f, doc, pub, pubTok, start, seenDecl), true
			}
		}
		return nil, false
	}
	next := p.peek(1)
	switch t.Text {
	case "func", "fun", "def", "function":
		if next.Kind != token.Ident && next.Kind != token.LParen {
			return nil, false
		}
		d := p.foreign(t.Span, "`%s` is %s syntax; Xo declares functions with `fn` (core 3.2)", t.Text, fnKeywords[t.Text])
		addFix(d, "write `fn`", p.replaceSpan(t.Span, "fn"))
		return p.parseFnDecl(doc, pub, start, false), true
	case "async":
		if next.Kind == token.Fn || (next.Kind == token.Ident && fnKeywords[next.Text] != "") {
			d := p.foreign(t.Span, "`async` is TypeScript, Python, and Rust syntax; Xo calls block the calling task, so functions need no `async` (core 7.1)")
			addFix(d, "remove `async`", p.deleteToken(t))
			p.next()
			return p.parseTopDecl(f, doc, pub, pubTok, start, seenDecl), true
		}
	case "export", "public":
		if !startsTopDecl(next) {
			return nil, false
		}
		lang := "TypeScript"
		if t.Text == "public" {
			lang = "Kotlin and Swift"
		}
		d := p.foreign(t.Span, "`%s` is %s syntax; Xo exports a declaration with `pub` (core 1.5)", t.Text, lang)
		if pub {
			addFix(d, "remove `"+t.Text+"`", p.deleteToken(t))
		} else {
			addFix(d, "write `pub`", p.replaceSpan(t.Span, "pub"))
		}
		p.next()
		return p.parseTopDecl(f, doc, true, t, start, seenDecl), true
	case "private", "internal", "fileprivate":
		if !startsTopDecl(next) {
			return nil, false
		}
		d := p.foreign(t.Span, "`%s` is Kotlin and Swift syntax; Xo declarations are private to their module unless marked `pub` (core 1.5)", t.Text)
		addFix(d, "remove `"+t.Text+"`", p.deleteToken(t))
		p.next()
		return p.parseTopDecl(f, doc, pub, pubTok, start, seenDecl), true
	case "package":
		if next.Kind != token.Ident || t.Span.Start.Col != 1 {
			return nil, false
		}
		d := p.foreign(t.Span, "`package %s` is Go syntax; an Xo module is its directory and has no package clause (core 1.1)", next.Text)
		addFix(d, "remove the package clause", p.replaceRange(t.Span.Start, p.nextLineStart(t.Span.End), ""))
		p.skipLine()
		p.lineDone = true
		return nil, true
	case "import", "from":
		return p.foreignImport(f), true
	case "impl":
		return p.parseImpl(doc, start), true
	case "trait", "protocol":
		if next.Kind != token.Ident {
			return nil, false
		}
		lang := "Rust"
		if t.Text == "protocol" {
			lang = "Swift"
		}
		d := p.foreign(t.Span, "`%s` is %s syntax; Xo declares an `interface`, and types satisfy it implicitly by having its methods (core 2.4)", t.Text, lang)
		addFix(d, "write `interface`", p.replaceSpan(t.Span, "interface"))
		return p.parseInterface(doc, pub, start), true
	case "class", "data", "object", "extension":
		if t.Text == "data" && !(next.Kind == token.Ident && next.Text == "class") {
			return nil, false
		}
		if next.Kind != token.Ident {
			return nil, false
		}
		p.errorf(diag.ForeignSyntax, t.Span, "`%s` is Kotlin, Swift, TypeScript, or Python syntax; Xo has no classes: declare a `struct` for the fields and methods at top level with a receiver, `fn (p: P) name() -> T` (core 2.2, 3.4)", t.Text)
		return &ast.BadDecl{Base: ast.Base{Sp: t.Span}}, true
	}
	return nil, false
}

// startsTopDecl reports whether t can begin a declaration after a
// visibility word.
func startsTopDecl(t token.Token) bool {
	switch t.Kind {
	case token.Fn, token.Struct, token.Enum, token.Interface, token.Type, token.Const:
		return true
	case token.Ident:
		switch t.Text {
		case "func", "fun", "function", "def", "async", "class", "trait", "protocol", "data":
			return true
		}
	}
	return false
}

// goImports maps imports of other languages to their Xo module. An empty
// value means the package has no Xo import (its functions are builtin or
// methods); a missing key means there is no direct equivalent.
var goImports = map[string]string{
	"encoding/json": "std/json",
	"json":          "std/json",
	"net/http":      "std/http",
	"http":          "std/http",
	"strings":       "",
	"strconv":       "",
	"errors":        "",
	"math":          "",
	"sort":          "",
	"unicode/utf8":  "",
	"fmt":           "",
	"os":            "",
	"time":          "",
	"sync":          "",
	"context":       "",
	"log":           "",
}

// foreignImport reports Go, Python, and TypeScript import statements and
// consumes them.
func (p *parser) foreignImport(f *ast.File) ast.Decl {
	t := p.tok
	startLine := t.Span.Start
	// Go: import "x" or import ( "x" "y" ); Python: import x, from x import y;
	// TypeScript: import { a } from "b".
	var paths []string
	endIdx := -1
	i := p.pos + 1
	if p.toks[i].Kind == token.LParen && t.Text == "import" {
		endIdx = p.matchClose(i)
		for j := i + 1; endIdx > 0 && j < endIdx; j++ {
			if p.toks[j].Kind == token.String {
				paths = append(paths, strTokText(p.toks[j]))
			}
		}
	} else {
		for j := i; j < len(p.toks) && p.toks[j].Kind != token.Newline && p.toks[j].Kind != token.EOF; j++ {
			endIdx = j
		}
		if endIdx >= i {
			first := p.toks[i]
			last := p.toks[endIdx]
			switch {
			case first.Kind == token.String:
				// Go: import "encoding/json"
				paths = append(paths, strTokText(first))
			case first.Kind == token.Ident && t.Text == "import" && endIdx == i:
				// Python: import json
				paths = append(paths, first.Text)
			case first.Kind == token.Star && last.Kind == token.String:
				// TypeScript: import * as json from "json"
				paths = append(paths, strTokText(last))
			}
		}
	}
	if endIdx < 0 {
		endIdx = p.pos
	}
	endTok := p.toks[endIdx]
	sp := t.Span.To(endTok.Span)
	d := p.foreign(sp, "`%s` is Go, Python, or TypeScript syntax; Xo imports a module with `use`, for example `use std/json` (core 4.1)", t.Text)
	if len(paths) > 0 && startLine.Col == 1 {
		var uses []string
		known := true
		for _, path := range paths {
			if len(path) > 0 && path[0] == '"' {
				path = strings.Trim(path, "\"")
			}
			xo, ok := goImports[path]
			if !ok {
				known = false
				break
			}
			if xo != "" {
				uses = append(uses, "use "+xo)
			}
		}
		if known {
			repl := ""
			if len(uses) > 0 {
				repl = strings.Join(uses, "\n") + "\n"
			}
			// Recover the imports, so the names they bind resolve.
			for _, u := range uses {
				f.Uses = append(f.Uses, &ast.UseDecl{Base: ast.Base{Sp: sp}, Path: strings.TrimPrefix(u, "use ")})
			}
			addFix(d, "replace with `use`", p.replaceRange(startLine, p.nextLineStart(endTok.Span.End), repl))
		} else {
			addNote(d, "the std modules are listed in spec/std.md; there is no direct equivalent of `%s`", strings.Join(paths, "`, `"))
		}
	}
	p.skipTo(endIdx + 1)
	p.skipLine()
	p.lineDone = true
	return nil
}

// strTokText is the literal text of a plain string token.
func strTokText(t token.Token) string {
	var b strings.Builder
	for _, part := range t.Parts {
		if !part.IsExpr {
			b.WriteString(part.Text)
		}
	}
	return b.String()
}

// parseImpl recovers a Rust `impl Type { fn m(&self) ... }` block as
// top level methods with a receiver named `self`. The fix rewrites the
// block when every function in it takes `self`.
func (p *parser) parseImpl(doc *ast.Doc, start token.Pos) ast.Decl {
	implTok := p.tok
	p.next() // impl
	if p.at(token.LBrack) {
		p.parseTypeParams()
	}
	typ := p.parseType()
	if p.at(token.For) {
		p.next()
		typ = p.parseType()
	}
	d := p.foreign(implTok.Span, "`impl` blocks are Rust syntax; Xo declares each method at top level with a receiver, `fn (self: %s) name() -> T`, and a type satisfies an interface implicitly by having its methods (core 3.4, 2.4)", p.spanText(typ.Span()))
	if !p.at(token.LBrace) {
		p.errorExpected("`{`")
		return &ast.BadDecl{Base: ast.Base{Sp: implTok.Span}}
	}
	open := p.tok
	typText := p.spanText(typ.Span())
	var edits []diag.Edit
	fixable := true
	// The header line `impl X {` and the closing `}` go away.
	edits = append(edits, p.replaceRange(p.lineStart(implTok.Span.Start), p.nextLineStart(open.Span.End), ""))
	var decls []*ast.FnDecl
	saved := p.inImpl
	p.inImpl = true
	p.parseMembers("method", func() {
		mstart := p.tok.Span.Start
		mdoc := p.docFor(mstart.Line)
		mpub := false
		if p.at(token.Pub) {
			mpub = true
			p.next()
		}
		if !p.at(token.Fn) {
			p.errorExpected("a method starting with `fn`")
			return
		}
		fnTok := p.tok
		fn := p.parseFnDecl(mdoc, mpub, mstart, false)
		if len(fn.Params) == 0 || !fn.Params[0].Self {
			fixable = false
			decls = append(decls, fn)
			return
		}
		self := fn.Params[0]
		recv := &ast.Receiver{Base: ast.Base{Sp: self.Sp}, Var: self.Var, Name: ident("self", self.Name.Sp), Type: typ}
		fn.Recv = recv
		fn.Params = fn.Params[1:]
		recvText := "(self: " + typText + ") "
		if self.Var {
			recvText = "(var self: " + typText + ") "
		}
		edits = append(edits, p.insertAt(fn.Name.Sp.Start, recvText))
		// Remove the self parameter and the comma after it.
		end := self.Sp.End
		if len(fn.Params) > 0 {
			end = fn.Params[0].Sp.Start
		}
		edits = append(edits, p.replaceRange(self.Sp.Start, end, ""))
		_ = fnTok
		decls = append(decls, fn)
	})
	p.inImpl = saved
	if p.toks[p.pos-1].Kind == token.RBrace {
		closeTok := p.toks[p.pos-1]
		edits = append(edits, p.replaceRange(p.lineStart(closeTok.Span.Start), p.nextLineStart(closeTok.Span.End), ""))
	} else {
		fixable = false
	}
	if fixable && len(decls) > 0 {
		addFix(d, "move the methods to top level with a `self` receiver", edits...)
	} else if d != nil {
		addNote(d, "functions without `self` become plain functions: call them as `name(...)`, not `%s::name(...)`", typText)
	}
	if len(decls) == 0 {
		return &ast.BadDecl{Base: ast.Base{Sp: implTok.Span}}
	}
	// Return the first method; the rest are added by the caller through
	// pendingDecls.
	p.pendingDecls = append(p.pendingDecls, decls[1:]...)
	return decls[0]
}

// foreignResult recognizes a result type written without `->` right after
// the parameter list: `fn f(): Int` (TypeScript, Kotlin, Python
// annotations), `fn f() Int` and `fn f() (Int, error)` (Go).
func (p *parser) foreignResult(fn *ast.FnDecl) {
	t := p.tok
	switch {
	case t.Kind == token.Colon && p.startsTypeAt(p.pos+1):
		d := p.foreign(t.Span, "`): T` is TypeScript, Kotlin, and Python syntax for a result type; Xo writes `-> T` (core 3.2)")
		repl := "->"
		if adjacent(p.toks[p.pos-1], t) {
			repl = " ->"
		}
		addFix(d, "write `->`", p.replaceSpan(t.Span, repl))
		p.next()
		fn.Result = p.parseTypeTop()
	case t.Kind == token.LParen && p.prevIs(token.RParen):
		// Go multiple results.
		start := t.Span.Start
		typ := p.parseTypeTop()
		tt, ok := typ.(*ast.TupleType)
		if ok && len(tt.Elems) >= 2 && isErrorType(tt.Elems[len(tt.Elems)-1]) {
			rest := tt.Elems[:len(tt.Elems)-1]
			text := p.spanText(rest[0].Span())
			if len(rest) > 1 {
				text = "(" + p.srcText(rest[0].Span().Start, rest[len(rest)-1].Span().End) + ")"
			}
			d := p.foreign(typ.Span(), "a Go result list ending in `error` is written `-> T ! Error` in Xo: the function returns `T` or fails with an error, and callers propagate it with `?` (core 2.6)")
			addFix(d, "write `-> "+text+" ! Error`", p.replaceRange(start, typ.Span().End, "-> "+text+" ! Error"))
			if len(rest) == 1 {
				fn.Result = rest[0]
			} else {
				fn.Result = &ast.TupleType{Base: ast.Base{Sp: typ.Span()}, Elems: rest}
			}
			fn.Err = &ast.ErrSpec{Base: ast.Base{Sp: tt.Elems[len(tt.Elems)-1].Span()}}
			return
		}
		d := p.foreign(typ.Span(), "a Go result type follows the parameters directly; Xo writes `-> T` (core 3.2)")
		addFix(d, "insert `->`", p.insertAt(start, "-> "))
		fn.Result = typ
	case (t.Kind == token.Ident && !p.atIdent("throws") && !p.atIdent("where")) || t.Kind == token.Star || t.Kind == token.LBrack:
		d := p.foreign(t.Span, "a Go result type follows the parameters directly; Xo writes `-> T` (core 3.2)")
		addFix(d, "insert `->`", p.insertAt(t.Span.Start, "-> "))
		fn.Result = p.parseTypeTop()
	}
}

// automaticBounds are interfaces every type with suitable fields already
// has (core 2.9, std.md 1), so a bound list may drop them; Rust's names
// for them are included.
var automaticBounds = map[string]bool{
	"Eq": true, "Hash": false, "Debug": true, "PartialEq": true, "Clone": true, "Copy": true,
	"Sized": true, "Send": true, "Sync": true, "Equatable": true, "Hashable": false,
}

// boundRename maps Rust and Swift bound names to their Xo interface.
var boundRename = map[string]string{"Hashable": "Hash", "Comparable": "Ord", "PartialOrd": "Ord", "Codable": "Json", "Serialize": "Json"}

// parseBounds handles a Rust or Swift bound list `T: A + B` after the first
// bound. Xo takes one interface per type parameter.
func (p *parser) parseBounds(name *ast.Ident, first ast.Type) ast.Type {
	if !p.at(token.Plus) || p.bad {
		return first
	}
	bounds := []ast.Type{first}
	for p.got(token.Plus) {
		bounds = append(bounds, p.parseType())
	}
	if p.bad {
		return first
	}
	sp := first.Span().To(bounds[len(bounds)-1].Span())
	var keep []string
	for _, b := range bounds {
		text := p.spanText(b.Span())
		if nt, ok := b.(*ast.NamedType); ok && len(nt.Path) == 1 && len(nt.Args) == 0 {
			if automaticBounds[text] {
				continue
			}
			if r, ok := boundRename[text]; ok {
				text = r
			}
		}
		dup := false
		for _, k := range keep {
			dup = dup || k == text
		}
		if !dup {
			keep = append(keep, text)
		}
	}
	all := p.spanText(sp)
	switch len(keep) {
	case 0:
		d := p.foreign(sp, "`%s: %s` is a Rust or Swift bound list; Xo takes one interface per type parameter, and `%s` are automatic for every type whose fields have them (core 2.7, 2.9): write `%s` with no constraint", name.Name, all, all, name.Name)
		addFix(d, "remove the bounds", p.replaceRange(name.Sp.End, sp.End, ""))
		return nil
	case 1:
		d := p.foreign(sp, "`%s: %s` is a Rust or Swift bound list; Xo takes one interface per type parameter, and `Eq` and `Debug` are automatic (core 2.7, 2.9): write `%s: %s`", name.Name, all, name.Name, keep[0])
		addFix(d, "write `"+name.Name+": "+keep[0]+"`", p.replaceSpan(sp, keep[0]))
		for _, b := range bounds {
			if p.spanText(b.Span()) == keep[0] {
				return b
			}
		}
		return &ast.NamedType{Base: ast.Base{Sp: sp}, Path: []*ast.Ident{ident(keep[0], sp)}}
	}
	p.foreign(sp, "`%s: %s` is a Rust or Swift bound list; Xo takes one interface per type parameter (core 2.7): declare an interface holding the methods the function needs and use it as the constraint", name.Name, all)
	return first
}

// foreignSelfParam recognizes Rust receivers `&self`, `&mut self`, and
// `mut self` in a parameter list. Inside a recovered `impl` block they are
// taken silently (the `impl` diagnostic covers them).
func (p *parser) foreignSelfParam(prm *ast.Param, allowSelf bool) bool {
	start := p.tok
	i := p.pos
	mut := false
	if p.toks[i].Kind == token.Amp {
		i++
	}
	if p.toks[i].Kind == token.Ident && p.toks[i].Text == "mut" {
		mut = true
		i++
	}
	if i == p.pos || p.toks[i].Kind != token.Ident || p.toks[i].Text != "self" {
		return false
	}
	selfTok := p.toks[i]
	if !p.inImpl {
		d := p.foreign(start.Span.To(selfTok.Span), "`%s` is Rust syntax; an Xo interface method takes a bare `self`, and a method declares its receiver before the name, `fn (var p: P) name()` (core 2.4, 3.4)", p.spanText(start.Span.To(selfTok.Span)))
		addFix(d, "write `self`", p.replaceRange(start.Span.Start, selfTok.Span.Start, ""))
	}
	p.skipTo(i + 1)
	prm.Self = true
	prm.Var = mut
	prm.Name = ident("self", selfTok.Span)
	_ = allowSelf
	return true
}

// foreignField recognizes Go fields without a colon (`x int`) and
// TypeScript optional fields (`x?: T`). It sets f.Type when it parsed the
// type itself.
func (p *parser) foreignField(f *ast.Field) bool {
	if p.bad {
		return false
	}
	t := p.tok
	switch {
	case t.Kind == token.Question && p.peek(1).Kind == token.Colon:
		q := t
		p.next()
		p.next()
		typ := p.parseType()
		d := p.foreign(q.Span, "`%s?: T` is TypeScript syntax for an optional field; Xo writes the option on the type, `%s: T?` (core 2.5)", f.Name.Name, f.Name.Name)
		addFix(d, "write `"+f.Name.Name+": "+p.spanText(typ.Span())+"?`", p.replaceSpan(q.Span, ""), p.insertAt(typ.Span().End, "?"))
		f.Type = &ast.OptionalType{Base: ast.Base{Sp: typ.Span()}, Elem: typ}
	case (t.Kind == token.Ident || t.Kind == token.Star || t.Kind == token.LBrack) && t.Span.Start.Line == f.Name.Sp.Start.Line:
		d := p.foreign(t.Span, "`%s %s` is Go syntax; Xo writes the field type after a colon, `%s: %s` (core 2.2)", f.Name.Name, t.Text, f.Name.Name, t.Text)
		addFix(d, "insert `:`", p.replaceRange(f.Name.Sp.End, t.Span.Start, ": "))
		f.Type = p.parseType()
		if p.at(token.String) || p.at(token.RawString) {
			// Go struct tags: `json:"name"`.
			tag := p.tok
			addNote(d, "Go struct tags have no Xo form: `derive Json` uses the field names (spec/std.md 5)")
			if d != nil && len(d.Fixes) > 0 {
				d.Fixes[0].Edits = append(d.Fixes[0].Edits, p.replaceRange(p.beforeBlanks(tag.Span.Start), tag.Span.End, ""))
			}
			p.next()
		}
	}
	return false
}

// prevIs reports whether the token before the current one has kind k.
func (p *parser) prevIs(k token.Kind) bool { return p.pos > 0 && p.toks[p.pos-1].Kind == k }

// startsTypeAt reports whether the token at index i can start a type.
func (p *parser) startsTypeAt(i int) bool {
	if i >= len(p.toks) {
		return false
	}
	switch p.toks[i].Kind {
	case token.Ident, token.LParen, token.Fn, token.LBrack, token.Star, token.Amp:
		return true
	}
	return false
}

// isErrorType reports whether t is Go's `error` (or Xo's `Error`).
func isErrorType(t ast.Type) bool {
	nt, ok := t.(*ast.NamedType)
	return ok && len(nt.Path) == 1 && len(nt.Args) == 0 && (nt.Path[0].Name == "error" || nt.Path[0].Name == "Error")
}

// foreignThrows reports Swift's `throws` after the parameters.
func (p *parser) foreignThrows(fn *ast.FnDecl, t token.Token) {
	d := p.foreign(t.Span, "`throws` is Swift syntax; an Xo function that can fail declares its error type after `!`: `-> T ! Error` (core 2.6)")
	if fn.Err != nil {
		addFix(d, "remove `throws`", p.replaceRange(p.beforeBlanks(t.Span.Start), t.Span.End, ""))
		return
	}
	if fn.Result == nil {
		addFix(d, "write `!`", p.replaceSpan(t.Span, "!"))
	} else {
		addFix(d, "move the failure to `! Error`",
			p.replaceRange(p.beforeBlanks(t.Span.Start), t.Span.End, ""),
			p.insertAt(fn.Result.Span().End, " ! Error"))
	}
	fn.Err = &ast.ErrSpec{Base: ast.Base{Sp: t.Span}}
}
