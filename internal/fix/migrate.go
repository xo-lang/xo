package fix

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/lexer"
	"github.com/xo-lang/xo/internal/token"
)

// Rewriter migrates code across one breaking change of a draft (or, later,
// an edition). A rewriter either rewrites source text (Text) or names the
// check diagnostics whose machine fixes perform the migration (Codes); the
// fix loop applies those, and the change report credits the rewriter.
type Rewriter struct {
	Name    string
	Draft   string // the draft that made the change, "0.3"
	Summary string
	Text    func(file string, src []byte) ([]byte, []Change)
	Codes   []string
}

// Registry lists the migration rewriters, oldest change first. They run in
// this order.
var Registry = []Rewriter{
	{Name: "variant-case", Draft: "0.2", Summary: "lowercase `ok err some none` become `Ok Err Some None`", Text: rewriteVariantCase},
	{Name: "scope-timeout", Draft: "0.2", Summary: "`scope s timeout d { ... }` becomes `task.with_timeout(clock, d, fn() { scope s { ... } })` when a `clock` is in the file", Text: rewriteScopeTimeout},
	{Name: "interp-dollar", Draft: "0.3", Summary: "string interpolation `{x}` becomes `${x}`", Text: rewriteInterp},
	{Name: "discard-reason", Draft: "0.4", Summary: "`let _ = <Result>` becomes `.ignore_err(\"TODO: reason\")` (fix of XO0206)", Codes: []string{string(diag.DiscardNeedsReason)}},
	{Name: "map-get-or-fault", Draft: "0.6", Summary: "Map index reads `m[k]` become `m.get_or_fault(k, \"TODO: ...\")` (fix of XO0410)", Codes: []string{string(diag.MapIndexRead)}},
}

// Select returns the rewriters for code written against draft from: every
// rewriter whose change came in a later draft. from is "draft-0.1", "0.1",
// or "v0.1". An empty from selects none.
func Select(from string) ([]Rewriter, error) {
	if from == "" {
		return nil, nil
	}
	v := strings.TrimPrefix(strings.TrimPrefix(from, "draft-"), "v")
	fv, err := draftNum(v)
	if err != nil {
		return nil, fmt.Errorf("unknown draft %q (want draft-0.N)", from)
	}
	var out []Rewriter
	for _, r := range Registry {
		rv, _ := draftNum(r.Draft)
		if rv > fv {
			out = append(out, r)
		}
	}
	return out, nil
}

// draftNum turns "0.N" into N.
func draftNum(v string) (int, error) {
	maj, minor, ok := strings.Cut(v, ".")
	if !ok || maj != "0" {
		return 0, fmt.Errorf("bad draft")
	}
	return strconv.Atoi(minor)
}

// lex tokenizes src, ignoring lexical errors (old drafts may not lex
// cleanly; the rewriters only look at tokens that did).
func lex(file string, src []byte) []token.Token {
	var ds diag.List
	return lexer.Lex(file, src, &ds).Tokens
}

type textEdit struct {
	start, end int
	text       string
}

func applyTextEdits(src []byte, es []textEdit) []byte {
	var b bytes.Buffer
	pos := 0
	for _, e := range es {
		if e.start < pos {
			continue
		}
		b.Write(src[pos:e.start])
		b.WriteString(e.text)
		pos = e.end
	}
	b.Write(src[pos:])
	return b.Bytes()
}

// rewriteVariantCase capitalizes the lowercase Result and optional variants
// of draft 0.1. `ok(`, `err(`, `some(` are rewritten when they are not a
// method (`.ok()`) or a declaration (`fn ok(`); bare `none` is rewritten
// unless it is being declared. A variable named `err` is left alone because
// it is not followed by `(`.
func rewriteVariantCase(file string, src []byte) ([]byte, []Change) {
	toks := lex(file, src)
	var es []textEdit
	var chs []Change
	for i, t := range toks {
		if t.Kind != token.Ident {
			continue
		}
		var prev token.Kind = token.Illegal
		if i > 0 {
			prev = toks[i-1].Kind
		}
		if prev == token.Dot || prev == token.Fn || prev == token.Let || prev == token.Var {
			continue
		}
		next := token.Illegal
		if i+1 < len(toks) {
			next = toks[i+1].Kind
		}
		var repl string
		switch t.Text {
		case "ok", "err", "some":
			if next == token.LParen {
				repl = strings.ToUpper(t.Text[:1]) + t.Text[1:]
			}
		case "none":
			if next != token.Colon {
				repl = "None"
			}
		}
		if repl == "" {
			continue
		}
		es = append(es, textEdit{t.Span.Start.Offset, t.Span.End.Offset, repl})
		chs = append(chs, Change{File: file, Line: t.Span.Start.Line, Rule: "variant-case", Title: fmt.Sprintf("`%s` to `%s`", t.Text, repl)})
	}
	return applyTextEdits(src, es), chs
}

// rewriteInterp turns `{expr}` interpolation in plain and triple quoted
// strings into `${expr}`. A brace counts as interpolation when its content
// starts like an expression (a letter, `_`, a digit, or `(`) and has no
// quote; JSON like text such as "{\"a\": 1}" is left alone.
func rewriteInterp(file string, src []byte) ([]byte, []Change) {
	toks := lex(file, src)
	var es []textEdit
	var chs []Change
	for _, t := range toks {
		if t.Kind != token.String {
			continue
		}
		start, end := t.Span.Start.Offset, t.Span.End.Offset
		if end > len(src) || start >= end {
			continue
		}
		s := src[start:end]
		n := 0
		for i := 0; i < len(s); i++ {
			switch s[i] {
			case '\\':
				i++
				continue
			case '{':
				if i > 0 && s[i-1] == '$' {
					continue
				}
				j := closeBrace(s, i)
				if j < 0 {
					continue
				}
				inner := s[i+1 : j]
				if len(inner) == 0 || bytes.ContainsAny(inner, "\"\\\n") {
					continue
				}
				c := inner[0]
				if !(c == '_' || c == '(' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
					continue
				}
				es = append(es, textEdit{start + i, start + i, "$"})
				n++
				i = j
			}
		}
		if n > 0 {
			chs = append(chs, Change{File: file, Line: t.Span.Start.Line, Rule: "interp-dollar", Title: fmt.Sprintf("%d interpolation(s) `{x}` to `${x}`", n)})
		}
	}
	return applyTextEdits(src, es), chs
}

// closeBrace returns the index of the `}` matching the `{` at i, or -1.
func closeBrace(s []byte, i int) int {
	depth := 0
	for j := i; j < len(s); j++ {
		switch s[j] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return j
			}
		}
	}
	return -1
}

// rewriteScopeTimeout replaces draft 0.1 `scope s timeout d { body }` with
// `task.with_timeout(clock, d, fn() { scope s { body } })`, adding `use
// std/task` when missing. It needs a Clock, so it only fires in files that
// already name `clock`; elsewhere the construct is left for a human (the
// parser reports it). The result is a `T ! Timed[E]`; the fix loop then
// offers `?` where the value is unused.
func rewriteScopeTimeout(file string, src []byte) ([]byte, []Change) {
	toks := lex(file, src)
	hasClock := false
	hasTaskUse := false
	for _, t := range toks {
		if t.Kind == token.Ident && t.Text == "clock" {
			hasClock = true
		}
		if t.Kind == token.ImportPath && t.Value == "std/task" {
			hasTaskUse = true
		}
	}
	var es []textEdit
	var chs []Change
	for i := 0; i+3 < len(toks); i++ {
		if toks[i].Kind != token.Scope || toks[i+1].Kind != token.Ident || toks[i+2].Kind != token.Ident || toks[i+2].Text != "timeout" {
			continue
		}
		// The duration runs to the `{` at paren depth 0.
		k, depth := i+3, 0
		for ; k < len(toks); k++ {
			switch toks[k].Kind {
			case token.LParen, token.LBrack:
				depth++
			case token.RParen, token.RBrack:
				depth--
			}
			if depth == 0 && toks[k].Kind == token.LBrace {
				break
			}
		}
		if k >= len(toks) || k == i+3 {
			continue
		}
		// Matching close brace.
		m, d := k, 0
		for ; m < len(toks); m++ {
			switch toks[m].Kind {
			case token.LBrace, token.HashLBrace:
				d++
			case token.RBrace:
				d--
			}
			if d == 0 {
				break
			}
		}
		if m >= len(toks) {
			continue
		}
		if !hasClock {
			chs = append(chs, Change{File: file, Line: toks[i].Span.Start.Line, Rule: "scope-timeout", Title: "skipped: no `clock` in the file; rewrite by hand to task.with_timeout"})
			continue
		}
		dur := string(src[toks[i+3].Span.Start.Offset:toks[k-1].Span.End.Offset])
		head := fmt.Sprintf("task.with_timeout(clock, %s, fn() { scope %s {", dur, toks[i+1].Text)
		es = append(es, textEdit{toks[i].Span.Start.Offset, toks[k].Span.End.Offset, head})
		es = append(es, textEdit{toks[m].Span.Start.Offset, toks[m].Span.End.Offset, "} })"})
		chs = append(chs, Change{File: file, Line: toks[i].Span.Start.Line, Rule: "scope-timeout", Title: "`scope " + toks[i+1].Text + " timeout` to `task.with_timeout`"})
		i = k
	}
	if len(es) == 0 {
		return src, chs
	}
	out := applyTextEdits(src, es)
	if !hasTaskUse {
		out = addUse(file, out, "std/task")
	}
	return out, chs
}

// addUse inserts `use path` after the last `use` line, or at the top.
func addUse(file string, src []byte, path string) []byte {
	toks := lex(file, src)
	at := 0
	for i, t := range toks {
		if t.Kind == token.Use {
			// End of that line.
			for j := i; j < len(toks); j++ {
				if toks[j].Kind == token.Newline {
					at = toks[j].Span.End.Offset
					break
				}
				at = toks[j].Span.End.Offset
			}
		}
	}
	line := "use " + path + "\n"
	if at > 0 && at <= len(src) && src[at-1] != '\n' {
		line = "\n" + line
	}
	out := append([]byte{}, src[:at]...)
	out = append(out, line...)
	return append(out, src[at:]...)
}
