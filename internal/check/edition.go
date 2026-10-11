package check

import (
	"fmt"
	"sort"
	"strings"

	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/token"
)

// CurrentEdition is the newest edition this toolchain knows (core 13). A
// module whose xo.mod has no `edition` line is on this edition. Until 1.0
// the only edition is `preview` (decision 0155); the owner names the first
// frozen edition at 1.0.
const CurrentEdition = "preview"

// Editions lists every released edition, oldest first. The compiler supports
// all of them (core 13). Tests may append a future edition temporarily.
var Editions = []string{"preview"}

// EditionAliases maps an old edition name to the edition it stands for.
// `edition 2026` was the name of `preview` before decision 0155; it keeps
// building during the 0.x releases, and `xo check` gives the fixable note
// XO1315 on it.
var EditionAliases = map[string]string{"2026": "preview"}

// ResolveEdition returns the edition that s names: a released edition, or
// the edition an alias stands for. ok is false for an unknown name.
func ResolveEdition(s string) (string, bool) {
	if a, ok := EditionAliases[s]; ok {
		s = a
	}
	if editionIndex(s) < 0 {
		return "", false
	}
	return s, true
}

// editionIndex is the position of e in Editions, or -1.
func editionIndex(e string) int {
	for i, x := range Editions {
		if x == e {
			return i
		}
	}
	return -1
}

// parseEdition validates the argument of an `edition` line in xo.mod. alias
// is true when s is an old name of the edition (EditionAliases).
func parseEdition(s string) (ed string, alias bool, err error) {
	if s == "" {
		return "", false, fmt.Errorf("edition line has no edition (want edition %s)", CurrentEdition)
	}
	e, ok := ResolveEdition(s)
	if !ok {
		return "", false, fmt.Errorf("edition %q is not supported by this toolchain (known editions: %s)", s, strings.Join(Editions, ", "))
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

// editionAlias is an `edition` line of an xo.mod that names an alias
// (`edition 2026`), reported once per xo.mod by reportEditionAliases.
type editionAlias struct {
	file      string // the xo.mod, as diagnostics name it
	line, col int    // the alias argument, 1 based, col in bytes
	alias     string
	edition   string
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
		sp := token.Span{File: a.file, Start: token.Pos{Line: a.line, Col: a.col}, End: token.Pos{Line: a.line, Col: a.col + len(a.alias)}}
		d := c.report(diag.Note, diag.EditionAlias, sp, "`edition %s` is the old name of `edition %s`", a.alias, a.edition)
		note(d, token.Span{}, "before 1.0 the only edition is `%s` (decision 0155); the alias keeps building during the 0.x releases", a.edition)
		fix(d, fmt.Sprintf("rewrite `edition %s` to `edition %s`", a.alias, a.edition), replaceEdit(sp, a.edition))
	}
}
