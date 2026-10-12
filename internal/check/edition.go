package check

import (
	"fmt"
	"sort"
	"strings"

	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/mod"
	"github.com/xo-lang/xo/internal/token"
)

// CurrentEdition is the newest edition this toolchain knows (core 13,
// mod.CurrentEdition): a module whose xo.mod has no `edition` line is on
// it. The edition names live in internal/mod, which reads xo.work.
const CurrentEdition = mod.CurrentEdition

// ResolveEdition returns the edition that s names (mod.ResolveEdition).
func ResolveEdition(s string) (string, bool) { return mod.ResolveEdition(s) }

func editionIndex(e string) int { return mod.EditionIndex(e) }

// parseEdition validates the argument of an `edition` line in xo.mod. alias
// is true when s is an old name of the edition (EditionAliases).
func parseEdition(s string) (ed string, alias bool, err error) {
	if s == "" {
		return "", false, fmt.Errorf("edition line has no edition (want edition %s)", CurrentEdition)
	}
	e, ok := ResolveEdition(s)
	if !ok {
		return "", false, fmt.Errorf("edition %q is not supported by this toolchain (known editions: %s)", s, strings.Join(mod.Editions, ", "))
	}
	return e, e != s, nil
}

// SupportedEdition reports whether e is a released edition or an alias of
// one.
func SupportedEdition(e string) bool {
	_, ok := ResolveEdition(e)
	return ok
}

// AtLeast reports whether module m is on edition e or later. A rule that
// changes at an edition boundary tests it so code on older editions keeps
// its meaning (core 13). Std and Go binding modules are always current.
func (m *Module) AtLeast(e string) bool {
	cur := m.Edition
	if cur == "" {
		cur = CurrentEdition
	}
	return editionIndex(cur) >= editionIndex(e)
}

// editionRule is a check that applies to modules on edition Since or later.
// The list is the hook through which an edition changes checker behavior;
// it is empty until an edition after `preview` changes a rule, and tests
// add entries to exercise the mechanism.
type editionRule struct {
	Since string
	Name  string
	Check func(c *checker, f *File)
}

var editionRules []editionRule

// runEditionRules applies the edition gated rules to the files of m.
func (c *checker) runEditionRules(m *Module) {
	for _, r := range editionRules {
		if !m.AtLeast(r.Since) {
			continue
		}
		for _, f := range m.Files {
			r.Check(c, f)
		}
	}
}

// editionAlias is an `edition` line of an xo.mod, or the `xo` line of an
// xo.work, that names an alias (`edition 2026`, `xo 2026`), reported once
// per file by reportEditionAliases.
type editionAlias struct {
	file      string // the xo.mod or xo.work, as diagnostics name it
	line, col int    // the alias argument, 1 based, col in bytes
	alias     string
	edition   string
	keyword   string // "edition" (xo.mod) or "xo" (xo.work); "" is "edition"
}

// workEditionAlias finds the `xo <alias>` line of the xo.work at name,
// read through the overlay (so the fix loop of `xo fix` sees its own
// rewrite), and registers it for reportEditionAliases. mod.ReadWork
// rejects an unknown edition (XO1301).
func (c *checker) workEditionAlias(name string) {
	data, err := c.readSource(name)
	if err != nil {
		return
	}
	for i, text := range strings.Split(string(data), "\n") {
		if j := strings.Index(text, "//"); j >= 0 {
			text = text[:j]
		}
		f := strings.Fields(text)
		if len(f) != 2 || f[0] != "xo" {
			continue
		}
		e, ok := mod.ResolveEdition(f[1])
		if !ok || e == f[1] {
			continue
		}
		file := relDir(name)
		k := strings.Index(text, "xo") + len("xo")
		col := k + strings.Index(text[k:], f[1]) + 1
		if c.editionAliases == nil {
			c.editionAliases = map[string]editionAlias{}
		}
		c.editionAliases[file] = editionAlias{file: file, line: i + 1, col: col, alias: f[1], edition: e, keyword: "xo"}
	}
}

// reportEditionAliases gives the fixable note XO1315 on every `edition`
// line of a checked module's xo.mod that uses an old edition name
// (decision 0155). The fix rewrites the name; `xo fix` applies it.
func (c *checker) reportEditionAliases() {
	files := make([]string, 0, len(c.editionAliases))
	for f := range c.editionAliases {
		files = append(files, f)
	}
	sort.Strings(files)
	for _, f := range files {
		a := c.editionAliases[f]
		kw := a.keyword
		if kw == "" {
			kw = "edition"
		}
		sp := token.Span{File: a.file, Start: token.Pos{Line: a.line, Col: a.col}, End: token.Pos{Line: a.line, Col: a.col + len(a.alias)}}
		d := c.report(diag.Note, diag.EditionAlias, sp, "`%s %s` is the old name of `%s %s`", kw, a.alias, kw, a.edition)
		note(d, token.Span{}, "before 1.0 the only edition is `%s` (decision 0155); the alias keeps building during the 0.x releases", a.edition)
		fix(d, fmt.Sprintf("rewrite `%s %s` to `%s %s`", kw, a.alias, kw, a.edition), replaceEdit(sp, a.edition))
	}
}
