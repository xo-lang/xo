package gen

import (
	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
)

// logFrom is the attribution of a log record written by the call at of
// the std/log function fo (decision 0173): "<module path>@<version>" when
// the call is in the source of a required module version (a dependency),
// "" otherwise (the main module, a workspace module, std, or not a log
// record function). The call's source file decides, not the function
// being generated, so a closure literal of the main module inlined into a
// dependency's specialized copy stays the main module's, and the reverse.
func (g *gen) logFrom(fo *check.FuncObj, at ast.Node) string {
	if at == nil || fo == nil || fo.Recv != nil || !fo.Module.IsStd || fo.Module.Path != "std/log" {
		return ""
	}
	switch fo.Name {
	case "debug", "info", "warn", "error":
	default:
		return ""
	}
	m := g.moduleOfFile(at.Span().File)
	if m == nil {
		return ""
	}
	return m.Dependency
}

// moduleOfFile is the loaded module whose source file is path.
func (g *gen) moduleOfFile(path string) *check.Module {
	if g.fileMods == nil {
		g.fileMods = map[string]*check.Module{}
		for _, m := range g.info.AllModules() {
			if m.IsStd {
				continue
			}
			for _, f := range m.Files {
				g.fileMods[f.Path] = m
			}
		}
	}
	return g.fileMods[path]
}
