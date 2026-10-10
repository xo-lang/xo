package parser

import (
	"strings"

	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/token"
)

// Lexical habits (decision 0087). The lexer stays byte for byte equal to
// its self hosted port (selfhost/lexer), so these are handled here, on the
// token stream, before parsing: `#` and `/* */` comments, `#[...]`
// attributes, `@Annotations`, template strings in backticks, Swift's
// `\(x)` interpolation, and Python's single quoted strings (in
// foreignPrimary). Each rewrite replaces the lexer's per character errors
// with one diagnostic and removes the foreign tokens, so the parser sees
// the rest of the line as Xo.

// lexHabits rewrites p.toks in place.
func (p *parser) lexHabits() {
	var out []token.Token
	for i := 0; i < len(p.toks); i++ {
		t := p.tokAt(i)
		switch {
		case t.Kind == token.Illegal && t.Text == "#":
			if j, ok := p.hashHabit(i); ok {
				out = p.dropRange(out, i, j)
				i = j - 1
				continue
			}
		case t.Kind == token.Illegal && t.Text == "@" && i+1 < len(p.toks) && p.tokAt(i+1).Kind == token.Ident && adjacent(t, p.tokAt(i+1)):
			j := i + 2
			if p.tokAt(j).Kind == token.LParen && adjacent(p.tokAt(j-1), p.tokAt(j)) {
				if c := p.matchClose(j); c > 0 {
					j = c + 1
				}
			}
			name := p.tokAt(i + 1).Text
			d := p.lexDiag(t.Span.Start)
			if d != nil {
				d.Message = "`@" + name + "` is a Kotlin, Swift, TypeScript, or Python annotation; Xo has none: tests are `test \"name\" { ... }` blocks (core 9.2), and derived behavior is `derive Json` after a type body (core 2.9)"
			}
			out = p.dropRange(out, i, j)
			i = j - 1
			continue
		case t.Kind == token.Illegal && t.Text == "`":
			if j, nt, ok := p.templateHabit(i); ok {
				out = append(out, nt)
				i = j - 1
				continue
			}
		case t.Kind == token.Slash && i+1 < len(p.toks) && p.tokAt(i+1).Kind == token.Star && adjacent(t, p.tokAt(i+1)):
			if j, ok := p.blockComment(i); ok {
				out = p.dropRange(out, i, j)
				i = j - 1
				continue
			}
		case t.Kind == token.String:
			p.swiftInterp(&t)
		case t.Kind == token.Newline && p.leadingOperator(i):
			// `a\n|| b`: read as `a || b` (XO0178)
			continue
		case (t.Kind == token.Plus || t.Kind == token.Minus) && i+2 < len(p.toks) && p.tokAt(i+1).Kind == t.Kind && adjacent(t, p.tokAt(i+1)) &&
			p.tokAt(i+2).Kind != token.Newline && p.tokAt(i+2).Span.Start.Line > t.Span.Start.Line:
			// `x++` at the end of a line: the lexer inserts no newline after
			// an operator, so add the one the Xo form `x += 1` would get.
			out = append(out, t, p.tokAt(i+1))
			end := p.tokAt(i + 1).Span.End
			out = append(out, token.Token{Kind: token.Newline, Span: token.Span{File: p.file, Start: end, End: end}, Text: "\n"})
			i++
			continue
		}
		out = append(out, t)
	}
	p.toks = out
}

// dropRange skips tokens [i, j) and keeps a statement ending newline when
// the removed text separated two lines.
func (p *parser) dropRange(out []token.Token, i, j int) []token.Token {
	if j >= len(p.toks) {
		j = len(p.toks) - 1
	}
	if len(out) == 0 {
		return out
	}
	prev := out[len(out)-1]
	next := p.tokAt(j)
	if prev.Kind != token.Newline && next.Kind != token.Newline && next.Kind != token.EOF &&
		next.Span.Start.Line > prev.Span.End.Line && endsStmt(prev.Kind) {
		sp := token.Span{File: p.file, Start: prev.Span.End, End: prev.Span.End}
		out = append(out, token.Token{Kind: token.Newline, Span: sp, Text: "\n"})
	}
	return out
}

// endsStmt mirrors the lexer's newline rule (core 1.3).
func endsStmt(k token.Kind) bool {
	switch k {
	case token.Ident, token.Int, token.Float, token.Duration, token.Rune, token.String,
		token.RawString, token.Bytes, token.ImportPath, token.Hole,
		token.RParen, token.RBrack, token.RBrace, token.Question,
		token.Return, token.Break, token.Continue:
		return true
	}
	return false
}

// lexDiag returns the lexer diagnostic that starts at pos, if any.
func (p *parser) lexDiag(pos token.Pos) *diag.Diagnostic {
	for k := range p.diags.Items {
		d := &p.diags.Items[k]
		if d.Span.Line == pos.Line && d.Span.Col == pos.Col && strings.HasPrefix(d.Code, "XO015") {
			return d
		}
	}
	return nil
}

// dropLexDiags removes the lexer diagnostics that start in [a, b), except
// the first one starting exactly at a, which it returns (nil if none).
// Pointers into p.diags.Items taken before the call are invalid after it.
func (p *parser) dropLexDiags(a, b token.Pos) *diag.Diagnostic {
	before := func(l1, c1, l2, c2 int) bool { return l1 < l2 || (l1 == l2 && c1 < c2) }
	items := p.diags.Items[:0]
	kept := -1
	for _, d := range p.diags.Items {
		if strings.HasPrefix(d.Code, "XO015") {
			if kept < 0 && d.Span.Line == a.Line && d.Span.Col == a.Col {
				kept = len(items)
				items = append(items, d)
				continue
			}
			if !before(d.Span.Line, d.Span.Col, a.Line, a.Col) && before(d.Span.Line, d.Span.Col, b.Line, b.Col) {
				continue
			}
		}
		items = append(items, d)
	}
	p.diags.Items = items
	if kept < 0 {
		return nil
	}
	return &p.diags.Items[kept]
}

// lineEnd is the index of the first token after i on a later line, or the
// newline token that ends i's line.
func (p *parser) lineEnd(i int) int {
	line := p.tokAt(i).Span.Start.Line
	j := i + 1
	for j < len(p.toks) && p.tokAt(j).Kind != token.EOF && p.tokAt(j).Span.Start.Line == line && p.tokAt(j).Kind != token.Newline {
		j++
	}
	return j
}

// hashHabit handles `# comment` (Python, shell), `#!` lines, and Rust's
// `#[attribute]`. It returns the index of the first token to keep.
func (p *parser) hashHabit(i int) (int, bool) {
	t := p.tokAt(i)
	off := t.Span.Start.Offset
	var c byte
	if off+1 < len(p.src) {
		c = p.src[off+1]
	}
	switch {
	case c == '[' || (c == '!' && off+2 < len(p.src) && p.src[off+2] == '['):
		// Rust attribute: up to the matching `]` on this line.
		k := i + 1
		for k < len(p.toks) && p.tokAt(k).Kind != token.LBrack {
			k++
		}
		e := p.matchClose(k)
		if e < 0 || p.tokAt(e).Span.Start.Line != t.Span.Start.Line {
			return 0, false
		}
		text := p.srcText(t.Span.Start, p.tokAt(e).Span.End)
		d := p.dropLexDiags(t.Span.Start, p.tokAt(e).Span.End)
		if d != nil {
			d.Message = "`" + text + "` is a Rust attribute; Xo has none: write `derive Display, Ord, Json` after a struct or enum body, and `Eq`, `Hash`, and `Debug` are automatic for every type whose fields have them (core 2.9)"
			if names, ok := deriveNames(text); ok {
				var keep []string
				for _, n := range names {
					if x, ok := rustDerives[n]; ok {
						if x != "" {
							keep = append(keep, x)
						}
						continue
					}
					keep = nil
					ok = false
					break
				}
				nx := e + 1
				for nx < len(p.toks)-1 && p.tokAt(nx).Kind == token.Newline {
					nx++
				}
				if len(keep) == 0 && strings.TrimSpace(p.srcText(p.lineStart(t.Span.Start), t.Span.Start)) == "" && p.tokAt(nx).Span.Start.Line != t.Span.Start.Line {
					d.Fixes = append(d.Fixes, diag.Fix{Title: "remove the attribute", Edits: []diag.Edit{p.replaceRange(p.lineStart(t.Span.Start), p.nextLineStart(p.tokAt(e).Span.End), "")}})
				} else if len(keep) > 0 {
					d.Notes = append(d.Notes, diag.NoteItem{Message: "write `derive " + strings.Join(keep, ", ") + "` after the closing `}` of the type"})
				}
			}
		}
		return e + 1, true
	case c == ' ' || c == '\t' || c == '!' || c == '#' || c == '\n' || c == '\r' || off+1 >= len(p.src):
		// A comment to the end of the line.
		end := p.nextLineStart(t.Span.Start)
		lineEnd := end
		if lineEnd.Offset > 0 && lineEnd.Offset <= len(p.src) && p.src[lineEnd.Offset-1] == '\n' {
			lineEnd = token.Pos{Line: t.Span.Start.Line, Col: t.Span.Start.Col + (lineEnd.Offset - 1 - off), Offset: lineEnd.Offset - 1}
		}
		d := p.dropLexDiags(t.Span.Start, lineEnd)
		if d != nil {
			d.Message = "`#` starts a comment in Python and shell; Xo comments start with `//` (core 1.2)"
			d.Fixes = append(d.Fixes, diag.Fix{Title: "write `//`", Edits: []diag.Edit{p.replaceSpan(t.Span, "//")}})
		}
		j := i + 1
		for j < len(p.toks) && p.tokAt(j).Kind != token.EOF && p.tokAt(j).Span.Start.Offset < lineEnd.Offset {
			j++
		}
		return j, true
	}
	return 0, false
}

// rustDerives maps Rust derive names to the Xo derive they need ("" when
// the behavior is automatic in Xo).
var rustDerives = map[string]string{
	"Debug": "", "Clone": "", "Copy": "", "PartialEq": "", "Eq": "", "Hash": "",
	"PartialOrd": "Ord", "Ord": "Ord", "Serialize": "Json", "Deserialize": "Json", "Display": "Display",
}

// deriveNames extracts the names of `#[derive(A, B)]`.
func deriveNames(text string) ([]string, bool) {
	i := strings.Index(text, "derive(")
	j := strings.LastIndex(text, ")")
	if i < 0 || j < i {
		return nil, false
	}
	var out []string
	seen := map[string]bool{}
	for _, n := range strings.Split(text[i+len("derive("):j], ",") {
		n = strings.TrimSpace(n)
		if k := strings.LastIndex(n, "::"); k >= 0 {
			n = n[k+2:]
		}
		if n != "" && !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	return out, len(out) > 0
}

// blockComment handles `/* ... */` (and `/** ... */`). It returns the index
// of the first token after the comment.
func (p *parser) blockComment(i int) (int, bool) {
	t := p.tokAt(i)
	off := t.Span.Start.Offset
	k := strings.Index(string(p.src[off+2:]), "*/")
	if k < 0 {
		return 0, false
	}
	endOff := off + 2 + k + 2
	end := posAt(p.src, t.Span.Start, endOff)
	p.dropLexDiags(t.Span.Start, end)
	d := p.diags.Errorf(diag.ForeignSyntax, token.Span{File: p.file, Start: t.Span.Start, End: end},
		"`/* ... */` is a block comment in Go, Rust, TypeScript, Kotlin, and Swift; Xo has only line comments: `//`, and `///` for doc comments (core 1.2)")
	// Fix: one `//` line per comment line when nothing follows `*/`.
	after := p.nextLineStart(end)
	rest := ""
	if after.Offset > endOff {
		rest = string(p.src[endOff:after.Offset])
	}
	before := p.srcText(p.lineStart(t.Span.Start), t.Span.Start)
	if strings.TrimSpace(rest) == "" {
		inner := string(p.src[off+2 : endOff-2])
		prefix := "// "
		if strings.HasPrefix(inner, "*") && !strings.HasPrefix(inner, "*/") {
			prefix = "/// "
			inner = inner[1:]
		}
		indent := ""
		if strings.TrimSpace(before) == "" {
			indent = before
		}
		var lines []string
		for _, ln := range strings.Split(inner, "\n") {
			ln = strings.TrimSpace(ln)
			ln = strings.TrimSpace(strings.TrimPrefix(ln, "*"))
			lines = append(lines, ln)
		}
		for len(lines) > 0 && lines[0] == "" {
			lines = lines[1:]
		}
		for len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
		var b strings.Builder
		for n, ln := range lines {
			if n > 0 {
				b.WriteString("\n" + indent)
			}
			b.WriteString(strings.TrimRight(prefix+ln, " "))
		}
		if len(lines) == 0 {
			b.WriteString("//")
		}
		d.Fixes = append(d.Fixes, diag.Fix{Title: "write `//` comments", Edits: []diag.Edit{p.replaceRange(t.Span.Start, end, b.String())}})
	}
	j := i
	for j < len(p.toks) && p.tokAt(j).Kind != token.EOF && p.tokAt(j).Span.Start.Offset < endOff {
		j++
	}
	return j, true
}

// posAt is the position of offset off, counting from pos.
func posAt(src []byte, pos token.Pos, off int) token.Pos {
	for i := pos.Offset; i < off && i < len(src); i++ {
		if src[i] == '\n' {
			pos.Line++
			pos.Col = 1
		} else {
			pos.Col++
		}
	}
	pos.Offset = off
	return pos
}

// templateHabit handles TypeScript template strings `...${x}...` in
// backticks: it builds the String token the Xo form would have produced.
func (p *parser) templateHabit(i int) (int, token.Token, bool) {
	t := p.tokAt(i)
	off := t.Span.Start.Offset
	// Find the closing backtick, skipping `${...}` and escapes.
	k := off + 1
	depth := 0
	closeOff := -1
	for k < len(p.src) {
		c := p.src[k]
		switch {
		case c == '\\':
			k += 2
			continue
		case c == '$' && k+1 < len(p.src) && p.src[k+1] == '{':
			depth++
			k += 2
			continue
		case c == '}' && depth > 0:
			depth--
		case c == '`' && depth == 0:
			closeOff = k
		}
		if closeOff >= 0 {
			break
		}
		k++
	}
	if closeOff < 0 {
		return 0, token.Token{}, false
	}
	endOff := closeOff + 1
	end := posAt(p.src, t.Span.Start, endOff)
	d := p.dropLexDiags(t.Span.Start, end)
	inner := string(p.src[off+1 : closeOff])
	if d != nil {
		d.Code = string(diag.ForeignSyntax)
		d.Message = "a template string in backticks is TypeScript syntax; every Xo string interpolates with `${x}`, so write it in double quotes: `\"...${x}...\"` (core 1.6)"
		if !strings.Contains(inner, "\n") {
			var b strings.Builder
			for n := 0; n < len(inner); n++ {
				if inner[n] == '"' {
					b.WriteByte('\\')
				}
				b.WriteByte(inner[n])
			}
			d.Fixes = append(d.Fixes, diag.Fix{Title: "write a double quoted string", Edits: []diag.Edit{p.replaceRange(t.Span.Start, end, `"`+b.String()+`"`)}})
		} else {
			d.Notes = append(d.Notes, diag.NoteItem{Message: "text that spans lines is a triple quoted string, `\"\"\"...\"\"\"` (core 1.6)"})
		}
	}
	// The String token: literal parts and `${...}` expressions.
	nt := token.Token{Kind: token.String, Span: token.Span{File: p.file, Start: t.Span.Start, End: end}, Text: string(p.src[off:endOff])}
	var lit strings.Builder
	n := 0
	for n < len(inner) {
		if inner[n] == '$' && n+1 < len(inner) && inner[n+1] == '{' {
			if lit.Len() > 0 {
				nt.Parts = append(nt.Parts, token.StrPart{Text: lit.String()})
				lit.Reset()
			}
			e := strings.IndexByte(inner[n:], '}')
			if e < 0 {
				break
			}
			exprOff := off + 1 + n + 2
			es := posAt(p.src, t.Span.Start, exprOff)
			ee := posAt(p.src, t.Span.Start, exprOff+e-2)
			nt.Parts = append(nt.Parts, token.StrPart{IsExpr: true, Text: inner[n+2 : n+e], Span: token.Span{File: p.file, Start: es, End: ee}})
			n += e + 1
			continue
		}
		if inner[n] == '\\' && n+1 < len(inner) {
			lit.WriteByte(inner[n+1])
			n += 2
			continue
		}
		lit.WriteByte(inner[n])
		n++
	}
	if lit.Len() > 0 {
		nt.Parts = append(nt.Parts, token.StrPart{Text: lit.String()})
	}
	j := i
	for j < len(p.toks) && p.tokAt(j).Kind != token.EOF && p.tokAt(j).Span.Start.Offset < endOff {
		j++
	}
	return j, nt, true
}

// swiftInterp handles Swift's `\(x)` inside a string: the lexer reported
// an unknown escape; this names the habit, adds the fix, and keeps the
// expressions as interpolated parts so the names in them count as used.
func (p *parser) swiftInterp(t *token.Token) {
	if !strings.Contains(t.Text, `\(`) || t.Triple {
		return
	}
	text := t.Text
	var edits []diag.Edit
	var first *diag.Diagnostic
	var firstSpan diag.Span
	for n := 0; n+1 < len(text); n++ {
		if text[n] == '\\' && text[n+1] != '(' {
			n++
			continue
		}
		if text[n] != '\\' {
			continue
		}
		depth := 0
		e := -1
		for k := n + 2; k < len(text); k++ {
			if text[k] == '(' {
				depth++
			} else if text[k] == ')' {
				if depth == 0 {
					e = k
					break
				}
				depth--
			}
		}
		if e < 0 {
			return
		}
		at := posAt(p.src, t.Span.Start, t.Span.Start.Offset+n)
		if d := p.lexDiag(at); d != nil && d.Code == string(diag.InvalidEscape) {
			d.Message = "`\\(x)` is Swift string interpolation; Xo writes `${x}` (core 1.6)"
			if first == nil {
				first = d
				firstSpan = d.Span
			} else {
				d.Code = "" // merged into the first
			}
		}
		edits = append(edits, p.replaceRange(at, posAt(p.src, t.Span.Start, t.Span.Start.Offset+n+2), "${"))
		closeAt := posAt(p.src, t.Span.Start, t.Span.Start.Offset+e)
		edits = append(edits, p.replaceRange(closeAt, posAt(p.src, t.Span.Start, t.Span.Start.Offset+e+1), "}"))
		es := posAt(p.src, t.Span.Start, t.Span.Start.Offset+n+2)
		t.Parts = append(t.Parts, token.StrPart{IsExpr: true, Text: text[n+2 : e], Span: token.Span{File: p.file, Start: es, End: closeAt}})
		n = e
	}
	// Drop the merged diagnostics.
	items := p.diags.Items[:0]
	for _, d := range p.diags.Items {
		if d.Code != "" {
			items = append(items, d)
		}
	}
	p.diags.Items = items
	if first != nil {
		for k := range p.diags.Items {
			if p.diags.Items[k].Span == firstSpan && p.diags.Items[k].Code == string(diag.InvalidEscape) {
				p.diags.Items[k].Fixes = append(p.diags.Items[k].Fixes, diag.Fix{Title: "write `${...}`", Edits: edits})
				break
			}
		}
	}
}
