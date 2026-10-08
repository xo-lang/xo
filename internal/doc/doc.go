// Package doc renders Markdown documentation of a module from its doc
// comments and signatures (`xo doc`, core 1.2 and 10.1).
//
// Doc comments are Markdown. Fenced code blocks in them with no info
// string or the info string `xo` are examples (core 1.2: `xo test` compiles
// and runs them); they are emitted as ```xo blocks and listed per item.
package doc

import (
	"fmt"
	"strings"

	"github.com/xo-lang/xo/internal/check"
	"github.com/xo-lang/xo/internal/query"
)

// Item is one documented declaration.
type Item struct {
	query.Def
	Examples []string `json:"examples"`
	Methods  []Item   `json:"methods,omitempty"`
}

// Module is the documentation of one module.
type Module struct {
	Module    string `json:"module"`
	Constants []Item `json:"constants"`
	Types     []Item `json:"types"`
	Functions []Item `json:"functions"`
}

// Build collects the documentation of m: pub declarations only unless all
// is set. Methods are grouped under their receiver type when the type is
// documented, and listed as functions otherwise.
func Build(s *query.Session, m *check.Module, all bool) *Module {
	out := &Module{Module: m.Path, Constants: []Item{}, Types: []Item{}, Functions: []Item{}}
	types := map[string]int{}
	var methods []Item
	for _, d := range s.Surface(m, all) {
		it := Item{Def: d, Examples: Examples(d.Doc)}
		if it.Examples == nil {
			it.Examples = []string{}
		}
		switch d.Kind {
		case "const":
			out.Constants = append(out.Constants, it)
		case "fn":
			out.Functions = append(out.Functions, it)
		case "method":
			methods = append(methods, it)
		default:
			types[d.Name] = len(out.Types)
			out.Types = append(out.Types, it)
		}
	}
	for _, it := range methods {
		recv := it.Name[:strings.IndexByte(it.Name, '.')]
		if i, ok := types[recv]; ok {
			out.Types[i].Methods = append(out.Types[i].Methods, it)
		} else {
			out.Functions = append(out.Functions, it)
		}
	}
	return out
}

// Examples returns the bodies of the example code blocks of a doc comment:
// fenced blocks with no info string or `xo`.
func Examples(doc string) []string {
	var out []string
	var cur []string
	in, example := false, false
	for _, ln := range strings.Split(doc, "\n") {
		t := strings.TrimSpace(ln)
		if !strings.HasPrefix(t, "```") {
			if in && example {
				cur = append(cur, ln)
			}
			continue
		}
		if !in {
			info := strings.TrimSpace(strings.TrimPrefix(t, "```"))
			in, example = true, info == "" || info == "xo"
			cur = nil
			continue
		}
		if example {
			out = append(out, strings.Join(cur, "\n")+"\n")
		}
		in = false
	}
	return out
}

// normalizeFences tags example fences ```xo so Markdown renderers
// highlight them.
func normalizeFences(doc string) string {
	lines := strings.Split(doc, "\n")
	in := false
	for i, ln := range lines {
		t := strings.TrimSpace(ln)
		if !strings.HasPrefix(t, "```") {
			continue
		}
		if !in && t == "```" {
			lines[i] = strings.Replace(ln, "```", "```xo", 1)
		}
		in = !in
	}
	return strings.Join(lines, "\n")
}

// Markdown renders the documentation.
func (d *Module) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Module `%s`\n", d.Module)
	n := len(d.Constants) + len(d.Types) + len(d.Functions)
	if n == 0 {
		b.WriteString("\nNo exported declarations.\n")
		return b.String()
	}
	b.WriteString("\n## Index\n\n")
	index := func(items []Item, kind string) {
		for _, it := range items {
			fmt.Fprintf(&b, "- %s [`%s`](#%s)\n", kind, it.Name, anchor(it.Name))
			for _, m := range it.Methods {
				fmt.Fprintf(&b, "  - method [`%s`](#%s)\n", m.Name, anchor(m.Name))
			}
		}
	}
	index(d.Constants, "const")
	index(d.Types, "type")
	index(d.Functions, "fn")
	section := func(title string, items []Item, level string) {
		if len(items) == 0 {
			return
		}
		fmt.Fprintf(&b, "\n## %s\n", title)
		for _, it := range items {
			writeItem(&b, it, level)
			for _, m := range it.Methods {
				writeItem(&b, m, level+"#")
			}
		}
	}
	section("Constants", d.Constants, "###")
	section("Types", d.Types, "###")
	section("Functions", d.Functions, "###")
	return b.String()
}

func writeItem(b *strings.Builder, it Item, level string) {
	fmt.Fprintf(b, "\n%s %s\n\n", level, it.Name)
	fmt.Fprintf(b, "```xo\n%s\n```\n", it.Signature)
	if it.Doc != "" {
		fmt.Fprintf(b, "\n%s\n", normalizeFences(it.Doc))
	}
	if len(it.Examples) > 0 {
		fmt.Fprintf(b, "\n%d example(s) (fenced `xo` blocks above).\n", len(it.Examples))
	}
	fmt.Fprintf(b, "\nDefined at %s.\n", it.Pos)
}

// anchor is the GitHub style heading anchor of a heading text.
func anchor(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_', r == '-':
			b.WriteRune(r)
		case r == ' ':
			b.WriteByte('-')
		}
	}
	return b.String()
}
