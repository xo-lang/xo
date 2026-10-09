// Package format prints Xo source in its one canonical layout (core 10.5,
// `xo fmt`). The layout is computed from the syntax tree; comments come
// from the lexer and are placed back between the nodes they sat between.
//
// Layout rules (there are no options):
//
//   - Indentation is 2 spaces per level; lines carry no trailing blanks.
//   - Statements, declarations, struct fields, enum variants, interface
//     methods, and match and select arms each start a line. Runs of blank
//     lines collapse to one; blank lines after `{` and before `}` are
//     dropped.
//   - A bracketed list (arguments, parameters, list, set, map, and struct
//     literals, tuples) is split one element per line, with a trailing
//     comma, when the source has a line break right after the opening
//     bracket; otherwise it is printed on one line without a trailing
//     comma.
//   - A block, struct, enum, interface, match, or select body stays on one
//     line only when it was on one line in the source and holds at most one
//     item: `fn(req) { http.json(200, x) }`, `struct P { x: Int, y: Int }`.
//   - Line breaks the grammar allows mid expression are kept where the
//     source has them: before a `.` in a method chain, after a binary
//     operator, after `=>` in an arm, before `->` and before each `uses`,
//     `requires`, `ensures`, and `else` clause of a function header.
//     Continuation lines are indented one level.
//   - `} else {`, `} else if`, and `} invariant`/`} derive` stay on the
//     line of the closing brace.
//   - Comments are kept. A comment that ended a code line stays at the end
//     of that line; trailing comments of consecutive lines with the same
//     indentation are aligned. Literals and interpolated strings are copied
//     from the source byte for byte.
package format

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/lexer"
	"github.com/xo-lang/xo/internal/parser"
)

// ParseError is returned when the source does not parse: only valid files
// are formatted.
type ParseError struct {
	Diagnostics []diag.Diagnostic
}

func (e *ParseError) Error() string {
	var msgs []string
	for _, d := range e.Diagnostics {
		msgs = append(msgs, diag.Human(d))
	}
	return "syntax errors:\n" + strings.Join(msgs, "\n")
}

// Source formats one file. A file that only parses as a signature listing
// (std/*.xo, functions without bodies) is formatted in that mode.
func Source(name string, src []byte) ([]byte, error) {
	f, ds := parser.ParseFile(name, src, 0)
	if len(ds) > 0 {
		f2, ds2 := parser.ParseFile(name, src, parser.SignatureMode)
		if len(ds2) > 0 {
			return nil, &ParseError{Diagnostics: ds}
		}
		f = f2
	}
	return File(f, src), nil
}

// File prints a parsed file. src must be the source f was parsed from.
func File(f *ast.File, src []byte) []byte {
	var lx diag.List
	res := lexer.Lex(f.Name, src, &lx)
	p := &printer{src: src, comments: res.Comments}
	p.file(f)
	return p.render()
}

// Decl prints one declaration of a file parsed from src in the canonical
// layout, without comments. With sig set a function's body is left out, so
// the result is its signature (used by `xo query def`, `xo query api`, and
// `xo doc`).
func Decl(d ast.Decl, src []byte, sig bool) string {
	if fd, ok := d.(*ast.FnDecl); ok && sig && fd.Body != nil {
		c := *fd
		c.Body = nil
		d = &c
	}
	p := &printer{src: src}
	p.decl(d)
	return strings.TrimRight(string(p.render()), "\n")
}

// line is one output line.
type line struct {
	indent  int
	raw     bool // continuation of a multi line string: printed verbatim
	noTrim  bool // ends inside a multi line string: keep trailing blanks
	text    string
	comment string // trailing comment
}

type printer struct {
	src      []byte
	comments []lexer.Comment
	ci       int // next comment to place

	out    []line
	cur    line
	indent int
	// lastOff is the source offset just past the last code or comment
	// printed; it decides whether a comment trails a code line and where
	// blank lines were.
	lastOff int
}

// ---------------------------------------------------------------------------
// Output

func (p *printer) w(s string) {
	if p.cur.text == "" && !p.cur.raw {
		p.cur.indent = p.indent
	}
	for {
		i := strings.IndexByte(s, '\n')
		if i < 0 {
			p.cur.text += s
			return
		}
		p.cur.text += s[:i]
		p.cur.noTrim = true
		p.out = append(p.out, p.cur)
		p.cur = line{raw: true}
		s = s[i+1:]
	}
}

// nl ends the current line if it holds anything.
func (p *printer) nl() {
	if p.cur.text != "" || p.cur.comment != "" || p.cur.raw {
		p.out = append(p.out, p.cur)
	}
	p.cur = line{}
}

// blank adds one empty line unless the output is empty or already ends in
// one.
func (p *printer) blank() {
	p.nl()
	if n := len(p.out); n == 0 || (p.out[n-1].text == "" && p.out[n-1].comment == "" && !p.out[n-1].raw) {
		return
	}
	p.out = append(p.out, line{})
}

func (p *printer) render() []byte {
	p.nl()
	// trim trailing blank lines
	for len(p.out) > 0 && p.out[len(p.out)-1].text == "" && p.out[len(p.out)-1].comment == "" && !p.out[len(p.out)-1].raw {
		p.out = p.out[:len(p.out)-1]
	}
	// align trailing comments of consecutive lines
	width := make([]int, len(p.out))
	for i := 0; i < len(p.out); {
		if !trailing(p.out[i]) {
			i++
			continue
		}
		j := i
		max := 0
		for j < len(p.out) && trailing(p.out[j]) && p.out[j].indent == p.out[i].indent && p.out[j].raw == p.out[i].raw {
			if n := utf8.RuneCountInString(p.out[j].text); n > max {
				max = n
			}
			j++
		}
		for k := i; k < j; k++ {
			width[k] = max
		}
		i = j
	}
	var b bytes.Buffer
	for i, ln := range p.out {
		if ln.text == "" && ln.comment == "" && !ln.raw {
			b.WriteByte('\n')
			continue
		}
		s := ln.text
		if !ln.raw {
			s = strings.Repeat("  ", ln.indent) + s
		}
		if ln.comment != "" {
			if ln.text != "" {
				s += strings.Repeat(" ", width[i]-utf8.RuneCountInString(ln.text)+1)
			}
			s += ln.comment
		}
		if !ln.noTrim && ln.comment == "" {
			s = strings.TrimRight(s, " \t")
		}
		b.WriteString(s)
		b.WriteByte('\n')
	}
	return b.Bytes()
}

func trailing(ln line) bool { return ln.comment != "" && ln.text != "" }

// ---------------------------------------------------------------------------
// Source helpers

func off(n ast.Node) int    { return n.Span().Start.Offset }
func endOff(n ast.Node) int { return n.Span().End.Offset }

// hasNL reports whether src[a:b] contains a line break.
func (p *printer) hasNL(a, b int) bool {
	if a < 0 || b > len(p.src) || a >= b {
		return false
	}
	return bytes.IndexByte(p.src[a:b], '\n') >= 0
}

// hasBlank reports whether src[a:b] contains an empty line.
func (p *printer) hasBlank(a, b int) bool {
	if a < 0 || b > len(p.src) || a >= b {
		return false
	}
	s := p.src[a:b]
	for {
		i := bytes.IndexByte(s, '\n')
		if i < 0 {
			return false
		}
		s = s[i+1:]
		j := 0
		for j < len(s) && (s[j] == ' ' || s[j] == '\t' || s[j] == '\r') {
			j++
		}
		if j < len(s) && s[j] == '\n' {
			return true
		}
	}
}

// find returns the offset of the first ch at or after from, skipping
// comments, or -1. It is only used between tokens whose text cannot hold
// ch.
func (p *printer) find(from int, ch byte) int {
	for i := from; i < len(p.src); i++ {
		c := p.src[i]
		if c == '/' && i+1 < len(p.src) && p.src[i+1] == '/' {
			for i < len(p.src) && p.src[i] != '\n' {
				i++
			}
			continue
		}
		if c == ch {
			return i
		}
	}
	return -1
}

// next returns the offset of the next significant byte at or after from,
// skipping blanks, comments, and commas.
func (p *printer) next(from int) int {
	i := from
	for i < len(p.src) {
		c := p.src[i]
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == ',':
			i++
		case c == '/' && i+1 < len(p.src) && p.src[i+1] == '/':
			for i < len(p.src) && p.src[i] != '\n' {
				i++
			}
		default:
			return i
		}
	}
	return len(p.src)
}

// hasComment reports whether a comment starts in [a, b).
func (p *printer) hasComment(a, b int) bool {
	for _, c := range p.comments[p.ci:] {
		o := c.Span.Start.Offset
		if o >= b {
			return false
		}
		if o >= a {
			return true
		}
	}
	return false
}

func (p *printer) text(n ast.Node) string {
	sp := n.Span()
	return string(p.src[sp.Start.Offset:sp.End.Offset])
}

// ---------------------------------------------------------------------------
// Comments and line starts

func (p *printer) commentText(c lexer.Comment) string {
	s := string(p.src[c.Span.Start.Offset:c.Span.End.Offset])
	return strings.TrimRight(s, " \t\r")
}

// flush places every comment that starts before limit. A comment on the
// line where the last code ended trails that line; others get their own
// lines at the current indentation. first suppresses a blank line before
// the first item of a body.
func (p *printer) flush(limit int, first bool) bool {
	for p.ci < len(p.comments) && p.comments[p.ci].Span.Start.Offset < limit {
		c := p.comments[p.ci]
		p.ci++
		start := c.Span.Start.Offset
		txt := p.commentText(c)
		if start >= p.lastOff && !p.hasNL(p.lastOff, start) && p.lastOff > 0 {
			if p.cur.text != "" && p.cur.comment == "" {
				p.cur.comment = txt
				p.nl()
				p.lastOff = c.Span.End.Offset
				continue
			}
			if n := len(p.out); p.cur.text == "" && !p.cur.raw && n > 0 && p.out[n-1].text != "" && p.out[n-1].comment == "" {
				p.out[n-1].comment = txt
				p.lastOff = c.Span.End.Offset
				continue
			}
		}
		p.nl()
		if !first && start >= p.lastOff && p.hasBlank(p.lastOff, start) {
			p.blank()
		}
		p.cur = line{indent: p.indent, text: txt}
		p.nl()
		first = false
		p.lastOff = c.Span.End.Offset
	}
	return first
}

// startLine begins a line for an item that starts at offset start: it
// places the comments before it, keeps one blank line where the source had
// one (unless first), and ends the current line.
func (p *printer) startLine(start int, first bool) {
	first = p.flush(start, first)
	p.nl()
	if !first && start >= p.lastOff && p.hasBlank(p.lastOff, start) {
		p.blank()
	}
}

// done records that code up to offset end has been printed.
func (p *printer) done(end int) {
	if end > p.lastOff {
		p.lastOff = end
	}
}

// ---------------------------------------------------------------------------
// Lists

// item is one element of a bracketed list.
type item struct {
	start, end int
	print      func()
}

// list prints open items close. openOff is the offset of the opening
// bracket in the source. The list is split one item per line when the
// source breaks the line right after the opening bracket (or holds a
// comment in an empty list). keepComma forces a trailing comma on one line
// (one element tuples).
func (p *printer) list(open, close string, openOff int, items []item, keepComma bool) {
	closeOff := -1
	if openOff >= 0 {
		from := openOff + len(open)
		if len(items) > 0 {
			from = items[len(items)-1].end
		}
		closeOff = p.next(from)
	}
	broken := false
	if openOff >= 0 {
		if len(items) > 0 {
			broken = p.hasNL(openOff+len(open), items[0].start)
		} else {
			broken = p.hasComment(openOff, closeOff)
		}
	}
	p.w(open)
	if !broken {
		for i, it := range items {
			if i > 0 {
				p.w(", ")
			}
			it.print()
			p.done(it.end)
		}
		if keepComma && len(items) == 1 {
			p.w(",")
		}
		p.w(close)
		if closeOff >= 0 {
			p.done(closeOff + 1)
		}
		return
	}
	p.done(openOff + len(open))
	p.indent++
	for i, it := range items {
		p.startLine(it.start, i == 0)
		it.print()
		p.w(",")
		p.done(it.end)
	}
	p.flush(closeOff, len(items) == 0)
	p.indent--
	p.nl()
	p.w(close)
	p.done(closeOff + 1)
}

// ---------------------------------------------------------------------------
// File and declarations

func (p *printer) file(f *ast.File) {
	var nodes []ast.Node
	for _, u := range f.Uses {
		nodes = append(nodes, u)
	}
	for _, d := range f.Decls {
		nodes = append(nodes, d)
	}
	for i, n := range nodes {
		p.startLine(off(n), i == 0)
		switch x := n.(type) {
		case *ast.UseDecl:
			p.useDecl(x)
		case ast.Decl:
			p.decl(x)
		}
		p.done(endOff(n))
	}
	p.flush(len(p.src)+1, len(nodes) == 0)
}

func (p *printer) useDecl(u *ast.UseDecl) {
	p.w("use ")
	if u.Go {
		p.w("go ")
		p.w(fmt.Sprintf("%q", u.Path))
	} else {
		p.w(u.Path)
	}
	if u.Alias != nil {
		p.w(" as " + u.Alias.Name)
	}
}

func (p *printer) decl(d ast.Decl) {
	switch x := d.(type) {
	case *ast.FnDecl:
		p.fnDecl(x)
	case *ast.StructDecl:
		p.structDecl(x)
	case *ast.EnumDecl:
		p.enumDecl(x)
	case *ast.InterfaceDecl:
		p.ifaceDecl(x)
	case *ast.TypeDecl:
		p.typeDecl(x)
	case *ast.ConstDecl:
		p.constDecl(x)
	case *ast.TestDecl:
		p.testDecl(x)
	default:
		p.w(p.text(d))
	}
}

func (p *printer) pub(b bool) {
	if b {
		p.w("pub ")
	}
}

// typeParams prints `[T, U: Ord, fx: Effects]` after a name ending at
// nameEnd and returns the offset just past the `]`.
func (p *printer) typeParams(tps []*ast.TypeParam, nameEnd int) int {
	if len(tps) == 0 {
		return nameEnd
	}
	open := p.find(nameEnd, '[')
	var items []item
	for _, tp := range tps {
		tp := tp
		items = append(items, item{off(tp), endOff(tp), func() {
			p.w(tp.Name.Name)
			switch {
			case tp.Effects:
				p.w(": Effects")
			case tp.Constraint != nil:
				p.w(": ")
				p.typ(tp.Constraint)
			}
		}})
	}
	p.list("[", "]", open, items, false)
	return p.lastOff
}

func (p *printer) params(ps []*ast.Param, openOff int) {
	var items []item
	for _, prm := range ps {
		prm := prm
		items = append(items, item{off(prm), endOff(prm), func() { p.param(prm) }})
	}
	p.list("(", ")", openOff, items, false)
}

func (p *printer) param(prm *ast.Param) {
	if prm.Var {
		p.w("var ")
	}
	p.w(prm.Name.Name)
	if prm.Type != nil {
		p.w(": ")
		p.typ(prm.Type)
	}
	if prm.Default != nil {
		p.w(" = ")
		p.expr(prm.Default)
	}
}

func (p *printer) fnDecl(fd *ast.FnDecl) {
	p.pub(fd.Pub)
	p.w("fn ")
	if fd.Recv != nil {
		p.w("(")
		if fd.Recv.Var {
			p.w("var ")
		}
		p.w(fd.Recv.Name.Name + ": ")
		p.typ(fd.Recv.Type)
		p.w(") ")
	}
	p.w(fd.Name.Name)
	p.done(endOff(fd.Name))
	end := p.typeParams(fd.TypeParams, endOff(fd.Name))
	p.params(fd.Params, p.find(end, '('))
	headerEnd := p.lastOff
	split := false
	base := p.indent
	cont := func(start int) {
		if p.hasNL(headerEnd, start) {
			p.indent = base + 1
			p.startLine(start, true)
			split = true
		} else {
			p.w(" ")
		}
	}
	if fd.Result != nil {
		cont(p.find(headerEnd, '-'))
		p.w("-> ")
		// a function type result takes a following `! E` or `uses`
		// (gotchas/fn-type-return-takes-uses.md): parenthesize it when
		// the declaration's own error or uses clause follows
		paren := false
		if ft, ok := fd.Result.(*ast.FnType); ok && len(ft.Uses) == 0 {
			usesNext := len(fd.Clauses) > 0 && fd.Err == nil
			if usesNext {
				_, usesNext = fd.Clauses[0].(*ast.UsesClause)
			}
			paren = (fd.Err != nil && ft.Err == nil) || usesNext
		}
		if paren {
			p.w("(")
		}
		p.typ(fd.Result)
		if paren {
			p.w(")")
		}
		headerEnd = endOff(fd.Result)
		p.done(headerEnd)
	}
	if fd.Err != nil {
		p.w(" ")
		p.errSpec(fd.Err)
		headerEnd = endOff(fd.Err)
		p.done(headerEnd)
	}
	for _, cl := range fd.Clauses {
		cont(off(cl))
		p.clause(cl)
		headerEnd = endOff(cl)
		p.done(headerEnd)
	}
	p.indent = base
	if fd.Body == nil {
		return
	}
	if split && p.hasNL(headerEnd, off(fd.Body)) {
		p.startLine(off(fd.Body), true)
	} else {
		p.w(" ")
	}
	p.block(fd.Body)
}

func (p *printer) clause(cl ast.Clause) {
	switch x := cl.(type) {
	case *ast.UsesClause:
		p.w("uses ")
		if x.Parens {
			p.w("(")
		}
		p.effects(x.Effects)
		if x.Parens {
			p.w(")")
		}
	case *ast.RequiresClause:
		p.w("requires")
		if x.Debug {
			p.w("(debug)")
		}
		p.w(" ")
		p.expr(x.Cond)
		if x.Else != nil {
			if p.hasNL(endOff(x.Cond), off(x.Else)) {
				saved := p.indent
				p.indent = saved + 1
				p.startLine(p.find(endOff(x.Cond), 'e'), true)
				p.w("else ")
				p.expr(x.Else)
				p.indent = saved
				return
			}
			p.w(" else ")
			p.expr(x.Else)
		}
	case *ast.EnsuresClause:
		p.w("ensures")
		if x.Debug {
			p.w("(debug)")
		}
		p.w(" ")
		p.expr(x.Cond)
	}
}

func (p *printer) effects(es []*ast.Effect) {
	for i, e := range es {
		if i > 0 {
			p.w(", ")
		}
		p.w(e.Name.Name)
		if e.Sub != nil {
			p.w("." + e.Sub.Name)
		}
	}
}

func (p *printer) errSpec(e *ast.ErrSpec) {
	p.w("!")
	if e.Type != nil {
		p.w(" ")
		p.typ(e.Type)
	}
}

// members prints a `{ ... }` body of declarations: struct fields, enum
// variants, interface methods. It stays on one line, `{ a: Int, b: Int }`,
// when the source body was on one line without comments.
func (p *printer) members(openOff int, items []item) {
	closeOff := p.next(openOff + 1)
	if len(items) > 0 {
		closeOff = p.next(items[len(items)-1].end)
	}
	if len(items) == 0 && !p.hasComment(openOff, closeOff) {
		p.w("{}")
		p.done(closeOff + 1)
		return
	}
	if !p.hasNL(openOff, closeOff) && !p.hasComment(openOff, closeOff) {
		p.w("{ ")
		for i, it := range items {
			if i > 0 {
				p.w(", ")
			}
			it.print()
			p.done(it.end)
		}
		p.w(" }")
		p.done(closeOff + 1)
		return
	}
	p.w("{")
	p.done(openOff + 1)
	p.indent++
	for i, it := range items {
		p.startLine(it.start, i == 0)
		it.print()
		p.done(it.end)
	}
	p.flush(closeOff, len(items) == 0)
	p.indent--
	p.nl()
	p.w("}")
	p.done(closeOff + 1)
}

func (p *printer) structDecl(s *ast.StructDecl) {
	p.pub(s.Pub)
	p.w("struct " + s.Name.Name)
	p.done(endOff(s.Name))
	end := p.typeParams(s.TypeParams, endOff(s.Name))
	p.w(" ")
	var items []item
	for _, f := range s.Fields {
		f := f
		items = append(items, item{off(f), endOff(f), func() {
			p.pub(f.Pub)
			p.w(f.Name.Name + ": ")
			p.typ(f.Type)
			if f.Default != nil {
				p.w(" = ")
				p.expr(f.Default)
			}
		}})
	}
	p.members(p.find(end, '{'), items)
	if s.Invariant != nil {
		p.w(" invariant ")
		p.expr(s.Invariant)
		p.done(endOff(s.Invariant))
	}
	p.derive(s.Derive)
}

func (p *printer) derive(ds []*ast.Ident) {
	if len(ds) == 0 {
		return
	}
	p.w(" derive ")
	for i, d := range ds {
		if i > 0 {
			p.w(", ")
		}
		p.w(d.Name)
	}
	p.done(endOff(ds[len(ds)-1]))
}

func (p *printer) enumDecl(e *ast.EnumDecl) {
	p.pub(e.Pub)
	p.w("enum " + e.Name.Name)
	p.done(endOff(e.Name))
	end := p.typeParams(e.TypeParams, endOff(e.Name))
	p.w(" ")
	var items []item
	for _, v := range e.Variants {
		v := v
		items = append(items, item{off(v), endOff(v), func() {
			p.w(v.Name.Name)
			p.done(endOff(v.Name))
			if v.Fields == nil {
				return
			}
			var fs []item
			for _, vf := range v.Fields {
				vf := vf
				fs = append(fs, item{off(vf), endOff(vf), func() {
					if vf.From {
						p.w("from ")
					} else {
						p.w(vf.Name.Name + ": ")
					}
					p.typ(vf.Type)
				}})
			}
			p.list("(", ")", p.find(endOff(v.Name), '('), fs, false)
		}})
	}
	p.members(p.find(end, '{'), items)
	p.derive(e.Derive)
}

func (p *printer) ifaceDecl(d *ast.InterfaceDecl) {
	p.pub(d.Pub)
	p.w("interface " + d.Name.Name)
	p.done(endOff(d.Name))
	end := p.typeParams(d.TypeParams, endOff(d.Name))
	p.w(" ")
	var items []item
	for _, m := range d.Methods {
		m := m
		items = append(items, item{off(m), endOff(m), func() { p.fnDecl(m) }})
	}
	p.members(p.find(end, '{'), items)
}

func (p *printer) typeDecl(d *ast.TypeDecl) {
	p.pub(d.Pub)
	p.w("type " + d.Name.Name)
	p.done(endOff(d.Name))
	p.typeParams(d.TypeParams, endOff(d.Name))
	if d.Newtype {
		p.w(" struct(")
		p.typ(d.Type)
		p.w(")")
		return
	}
	p.w(" = ")
	p.typ(d.Type)
}

func (p *printer) constDecl(c *ast.ConstDecl) {
	p.pub(c.Pub)
	p.w("const " + c.Name.Name)
	if c.Type != nil {
		p.w(": ")
		p.typ(c.Type)
	}
	p.w(" = ")
	p.expr(c.Value)
}

func (p *printer) testDecl(t *ast.TestDecl) {
	p.w("test")
	if t.Name != nil {
		p.w(" " + p.text(t.Name))
	}
	for i, prm := range t.For {
		if i == 0 {
			p.w(" for ")
		} else {
			p.w(", ")
		}
		p.param(prm)
	}
	if t.Seed != nil {
		p.w(" seed " + t.Seed.Raw)
	}
	if len(t.Uses) > 0 {
		p.w(" uses ")
		p.effects(t.Uses)
	}
	if t.Body != nil {
		p.w(" ")
		p.block(t.Body)
	}
}

// ---------------------------------------------------------------------------
// Types

func (p *printer) typ(t ast.Type) {
	switch x := t.(type) {
	case *ast.NamedType:
		for i, id := range x.Path {
			if i > 0 {
				p.w(".")
			}
			p.w(id.Name)
		}
		if len(x.Args) > 0 {
			p.w("[")
			for i, a := range x.Args {
				if i > 0 {
					p.w(", ")
				}
				p.typ(a)
			}
			p.w("]")
		}
	case *ast.OptionalType:
		if _, ok := x.Elem.(*ast.FnType); ok {
			p.w("(")
			p.typ(x.Elem)
			p.w(")")
		} else {
			p.typ(x.Elem)
		}
		p.w("?")
	case *ast.TupleType:
		p.w("(")
		for i, e := range x.Elems {
			if i > 0 {
				p.w(", ")
			}
			p.typ(e)
		}
		if len(x.Elems) == 1 {
			p.w(",")
		}
		p.w(")")
	case *ast.FnType:
		p.w("fn(")
		for i, fp := range x.Params {
			if i > 0 {
				p.w(", ")
			}
			if fp.Var {
				p.w("var ")
			}
			p.typ(fp.Type)
		}
		p.w(")")
		if x.Result != nil {
			p.w(" -> ")
			p.typ(x.Result)
		}
		if x.Err != nil {
			p.w(" ")
			p.errSpec(x.Err)
		}
		if len(x.Uses) > 0 {
			p.w(" uses ")
			if x.UsesParens || len(x.Uses) > 1 {
				p.w("(")
				p.effects(x.Uses)
				p.w(")")
			} else {
				p.effects(x.Uses)
			}
		}
	default:
		p.w(p.text(t))
	}
}

// ---------------------------------------------------------------------------
// Statements

// block prints `{ ... }`. A body that was on one line in the source with at
// most one statement and no comments stays on one line.
func (p *printer) block(b *ast.Block) {
	start, end := off(b), endOff(b)
	if len(b.Stmts) <= 1 && !p.hasNL(start, end) && !p.hasComment(start, end) {
		if len(b.Stmts) == 0 {
			p.w("{}")
		} else {
			p.w("{ ")
			p.stmt(b.Stmts[0])
			p.w(" }")
		}
		p.done(end)
		return
	}
	p.w("{")
	p.done(start + 1)
	p.indent++
	for i, s := range b.Stmts {
		p.startLine(off(s), i == 0)
		p.stmt(s)
		p.done(endOff(s))
	}
	p.flush(end-1, len(b.Stmts) == 0)
	p.indent--
	p.nl()
	p.w("}")
	p.done(end)
}

func (p *printer) label(l *ast.Ident) {
	if l != nil {
		p.w(l.Name + ": ")
	}
}

func (p *printer) stmt(s ast.Stmt) {
	switch x := s.(type) {
	case *ast.LetStmt:
		p.w("let ")
		p.pattern(x.Pattern)
		if x.Type != nil {
			p.w(": ")
			p.typ(x.Type)
		}
		p.w(" = ")
		p.expr(x.Value)
	case *ast.VarStmt:
		p.w("var " + x.Name.Name)
		if x.Type != nil {
			p.w(": ")
			p.typ(x.Type)
		}
		p.w(" = ")
		p.expr(x.Value)
	case *ast.AssignStmt:
		p.expr(x.Target)
		p.w(" " + x.Op.String() + " ")
		p.expr(x.Value)
	case *ast.DeferStmt:
		p.w("defer ")
		p.stmt(x.Body)
	case *ast.ReturnStmt:
		p.w("return")
		if x.Value != nil {
			p.w(" ")
			p.expr(x.Value)
		}
	case *ast.BreakStmt:
		p.w("break")
		if x.Label != nil {
			p.w(" " + x.Label.Name)
		}
	case *ast.ContinueStmt:
		p.w("continue")
		if x.Label != nil {
			p.w(" " + x.Label.Name)
		}
	case *ast.ForStmt:
		p.label(x.Label)
		p.w("for ")
		p.pattern(x.Pattern)
		p.w(" in ")
		p.expr(x.Iter)
		p.w(" ")
		p.block(x.Body)
	case *ast.WhileStmt:
		p.label(x.Label)
		p.w("while ")
		if x.Cond != nil {
			p.expr(x.Cond)
			p.w(" ")
		}
		p.block(x.Body)
	case *ast.ExprStmt:
		p.expr(x.X)
	case *ast.ConstDecl:
		p.constDecl(x)
	default:
		p.w(p.text(s))
	}
}

// ---------------------------------------------------------------------------
// Expressions

func (p *printer) expr(e ast.Expr) {
	switch x := e.(type) {
	case *ast.Ident:
		p.w(x.Name)
	case *ast.IntLit, *ast.FloatLit, *ast.DurationLit, *ast.RuneLit, *ast.StringLit, *ast.BytesLit:
		p.w(p.text(x))
	case *ast.HoleExpr:
		p.w("???")
	case *ast.UnitExpr:
		p.w("()")
	case *ast.ParenExpr:
		p.w("(")
		p.expr(x.X)
		p.w(")")
	case *ast.TupleExpr:
		p.list("(", ")", off(x), p.exprItems(x.Elems), true)
	case *ast.ListLit:
		p.list("[", "]", off(x), p.exprItems(x.Elems), false)
	case *ast.SetLit:
		p.list("#{", "}", off(x), p.exprItems(x.Elems), false)
	case *ast.MapLit:
		var items []item
		for _, en := range x.Entries {
			en := en
			items = append(items, item{off(en), endOff(en), func() {
				p.expr(en.Key)
				p.w(": ")
				p.expr(en.Value)
			}})
		}
		p.list("{", "}", off(x), items, false)
	case *ast.StructLit:
		p.expr(x.Type)
		p.fieldInits(x.Fields, p.find(endOff(x.Type), '{'))
	case *ast.UnaryExpr:
		p.w(x.Op.String())
		if u, ok := x.X.(*ast.UnaryExpr); ok && u.Op == x.Op {
			p.w(" ")
		}
		p.expr(x.X)
	case *ast.BinaryExpr:
		p.binary(x)
	case *ast.IsExpr:
		p.expr(x.X)
		p.w(" is ")
		p.pattern(x.Pattern)
	case *ast.RangeExpr:
		if x.Lo != nil {
			p.expr(x.Lo)
		}
		if x.Inclusive {
			p.w("..=")
		} else {
			p.w("..")
		}
		if x.Hi != nil {
			p.expr(x.Hi)
		}
	case *ast.SelectorExpr, *ast.CallExpr, *ast.TypeArgsExpr, *ast.IndexExpr, *ast.TryExpr, *ast.WithExpr:
		p.postfix(e)
	case *ast.IfExpr:
		p.ifExpr(x)
	case *ast.MatchExpr:
		p.w("match ")
		p.expr(x.Subject)
		p.w(" ")
		var arms []item
		for _, a := range x.Arms {
			a := a
			arms = append(arms, item{off(a), endOff(a), func() { p.matchArm(a) }})
		}
		p.arms(p.find(endOff(x.Subject), '{'), endOff(x), arms)
	case *ast.ScopeExpr:
		p.w("scope " + x.Name.Name + " ")
		p.block(x.Body)
	case *ast.SelectExpr:
		p.w("select ")
		var arms []item
		for _, a := range x.Arms {
			a := a
			arms = append(arms, item{off(a), endOff(a), func() { p.selectArm(a) }})
		}
		p.arms(p.find(off(x)+len("select"), '{'), endOff(x), arms)
	case *ast.Block:
		p.block(x)
	case *ast.FnLit:
		p.fnLit(x)
	case *ast.WhileExpr:
		p.w("while ")
		p.block(x.Body)
	default:
		p.w(p.text(e))
	}
	p.done(endOff(e))
}

func (p *printer) exprItems(es []ast.Expr) []item {
	var items []item
	for _, e := range es {
		e := e
		items = append(items, item{off(e), endOff(e), func() { p.expr(e) }})
	}
	return items
}

func (p *printer) fieldInits(fs []*ast.FieldInit, openOff int) {
	var items []item
	for _, f := range fs {
		f := f
		items = append(items, item{off(f), endOff(f), func() {
			p.w(f.Name.Name + ": ")
			p.expr(f.Value)
		}})
	}
	p.list("{", "}", openOff, items, false)
}

// binary prints `x op y`, keeping a line break after the operator where
// the source has one; the continuation is indented one level.
func (p *printer) binary(x *ast.BinaryExpr) {
	p.expr(x.X)
	p.w(" " + x.Op.String())
	if p.hasNL(endOff(x.X), off(x.Y)) {
		p.indent++
		p.startLine(off(x.Y), true)
		p.expr(x.Y)
		p.indent--
		return
	}
	p.w(" ")
	p.expr(x.Y)
}

// postfix prints a chain of selectors, calls, indexes, type arguments,
// `?`, and `with`. A `.name` that starts a line in the source starts a
// line here too, one level deeper than the chain's first line.
func (p *printer) postfix(e ast.Expr) {
	var ops []ast.Expr
	base := e
loop:
	for {
		switch x := base.(type) {
		case *ast.SelectorExpr:
			ops = append(ops, x)
			base = x.X
		case *ast.CallExpr:
			ops = append(ops, x)
			base = x.Fun
		case *ast.TypeArgsExpr:
			ops = append(ops, x)
			base = x.X
		case *ast.IndexExpr:
			ops = append(ops, x)
			base = x.X
		case *ast.TryExpr:
			ops = append(ops, x)
			base = x.X
		case *ast.WithExpr:
			ops = append(ops, x)
			base = x.X
		default:
			break loop
		}
	}
	p.expr(base)
	saved := p.indent
	broke := false
	for i := len(ops) - 1; i >= 0; i-- {
		switch x := ops[i].(type) {
		case *ast.SelectorExpr:
			if p.hasNL(endOff(x.X), off(x.Sel)) {
				if !broke {
					p.indent++
					broke = true
				}
				p.startLine(off(x.Sel), true)
			}
			p.w("." + x.Sel.Name)
			p.done(endOff(x.Sel))
		case *ast.CallExpr:
			var items []item
			for _, a := range x.Args {
				a := a
				items = append(items, item{off(a), endOff(a), func() {
					if a.Name != nil {
						p.w(a.Name.Name + ": ")
					}
					p.expr(a.Value)
				}})
			}
			p.list("(", ")", p.find(endOff(x.Fun), '('), items, false)
		case *ast.TypeArgsExpr:
			p.w("[")
			for j, t := range x.Types {
				if j > 0 {
					p.w(", ")
				}
				p.typ(t)
			}
			p.w("]")
		case *ast.IndexExpr:
			p.w("[")
			p.expr(x.Index)
			p.w("]")
		case *ast.TryExpr:
			p.w("?")
		case *ast.WithExpr:
			p.w(" with ")
			p.fieldInits(x.Fields, p.find(endOff(x.X), '{'))
		}
		p.done(endOff(ops[i]))
	}
	p.indent = saved
}

func (p *printer) ifExpr(x *ast.IfExpr) {
	p.w("if ")
	if x.Pattern != nil {
		p.w("let ")
		p.pattern(x.Pattern)
		p.w(" = ")
		p.expr(x.Value)
	} else {
		p.expr(x.Cond)
	}
	p.w(" ")
	p.block(x.Then)
	switch e := x.Else.(type) {
	case nil:
	case *ast.IfExpr:
		p.w(" else ")
		p.ifExpr(e)
	case *ast.Block:
		p.w(" else ")
		p.block(e)
	}
}

// arms prints the `{ arms }` of a match or select.
func (p *printer) arms(openOff, end int, arms []item) {
	closeOff := end - 1
	if len(arms) <= 1 && !p.hasNL(openOff, end) && !p.hasComment(openOff, end) {
		if len(arms) == 0 {
			p.w("{}")
		} else {
			p.w("{ ")
			arms[0].print()
			p.w(" }")
		}
		p.done(end)
		return
	}
	p.w("{")
	p.done(openOff + 1)
	p.indent++
	for i, a := range arms {
		p.startLine(a.start, i == 0)
		a.print()
		p.done(a.end)
	}
	p.flush(closeOff, len(arms) == 0)
	p.indent--
	p.nl()
	p.w("}")
	p.done(end)
}

// armBody prints ` => body`, keeping a line break after `=>`.
func (p *printer) armBody(headEnd int, body ast.Stmt) {
	p.w(" =>")
	if p.hasNL(headEnd, off(body)) {
		p.indent++
		p.startLine(off(body), true)
		p.stmt(body)
		p.indent--
		return
	}
	p.w(" ")
	p.stmt(body)
}

func (p *printer) matchArm(a *ast.MatchArm) {
	p.pattern(a.Pattern)
	head := endOff(a.Pattern)
	if a.Guard != nil {
		p.w(" if ")
		p.expr(a.Guard)
		head = endOff(a.Guard)
	}
	p.done(head)
	if a.Body != nil {
		p.armBody(p.find(head, '=')+2, a.Body)
	}
}

func (p *printer) selectArm(a *ast.SelectArm) {
	var head int
	switch {
	case a.After != nil:
		p.w("after ")
		p.expr(a.After)
		head = endOff(a.After)
	case a.Bind != nil:
		p.w(a.Bind.Name + " = ")
		p.expr(a.Op)
		head = endOff(a.Op)
	default:
		p.expr(a.Op)
		head = endOff(a.Op)
	}
	p.done(head)
	if a.Body != nil {
		p.armBody(p.find(head, '=')+2, a.Body)
	}
}

func (p *printer) fnLit(f *ast.FnLit) {
	p.w("fn")
	var items []item
	for _, prm := range f.Params {
		prm := prm
		items = append(items, item{off(prm), endOff(prm), func() { p.param(prm) }})
	}
	p.list("(", ")", p.find(off(f)+2, '('), items, false)
	if f.Result != nil {
		p.w(" -> ")
		p.typ(f.Result)
	}
	if f.Err != nil {
		p.w(" ")
		p.errSpec(f.Err)
	}
	if len(f.Uses) > 0 {
		p.w(" uses ")
		p.effects(f.Uses)
	}
	p.w(" ")
	p.block(f.Body)
}

// ---------------------------------------------------------------------------
// Patterns

func (p *printer) pattern(pt ast.Pattern) {
	switch x := pt.(type) {
	case *ast.WildcardPat:
		p.w("_")
	case *ast.BindPat:
		p.w(x.Name.Name)
	case *ast.LitPat:
		p.expr(x.Value)
	case *ast.RangePat:
		p.expr(x.Lo)
		if x.Inclusive {
			p.w("..=")
		} else {
			p.w("..")
		}
		p.expr(x.Hi)
	case *ast.OrPat:
		for i, a := range x.Alts {
			if i > 0 {
				p.w(" | ")
			}
			p.pattern(a)
		}
	case *ast.TuplePat:
		p.w("(")
		for i, e := range x.Elems {
			if i > 0 {
				p.w(", ")
			}
			p.pattern(e)
		}
		if len(x.Elems) == 1 {
			p.w(",")
		}
		p.w(")")
	case *ast.VariantPat:
		p.path(x.Path)
		if x.Parens {
			p.w("(")
			p.patFields(x.Fields, x.Rest)
			p.w(")")
		}
	case *ast.StructPat:
		p.path(x.Path)
		p.w("{")
		p.patFields(x.Fields, x.Rest)
		p.w("}")
	case *ast.ListPat:
		p.w("[")
		first := true
		sep := func() {
			if !first {
				p.w(", ")
			}
			first = false
		}
		for i, e := range x.Elems {
			if i == x.Rest {
				sep()
				p.w("..")
			}
			sep()
			p.pattern(e)
		}
		if x.Rest == len(x.Elems) {
			sep()
			p.w("..")
		}
		p.w("]")
	default:
		p.w(p.text(pt))
	}
	p.done(endOff(pt))
}

func (p *printer) path(ids []*ast.Ident) {
	for i, id := range ids {
		if i > 0 {
			p.w(".")
		}
		p.w(id.Name)
	}
}

func (p *printer) patFields(fs []*ast.PatField, rest bool) {
	for i, f := range fs {
		if i > 0 {
			p.w(", ")
		}
		if f.Name != nil {
			p.w(f.Name.Name + ": ")
		}
		p.pattern(f.Pattern)
	}
	if rest {
		if len(fs) > 0 {
			p.w(", ")
		}
		p.w("..")
	}
}
