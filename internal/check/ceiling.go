package check

import (
	"sort"

	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/mod"
	"github.com/xo-lang/xo/internal/token"
)

// ceilingState caches what the effect ceiling check (decision 0086 step
// 3) needs: the module root of each checked module and the ceilings
// declared by each module root.
type ceilingState struct {
	roots    map[*Module]modRoot
	noRoot   map[*Module]bool
	ceilings map[string]map[string][]mod.CeilingSource // root dir -> dependency path
}

// moduleRootOf is the xo.mod root holding module m.
func (c *checker) moduleRootOf(m *Module) (modRoot, bool) {
	cs := &c.ceil
	if cs.roots == nil {
		cs.roots, cs.noRoot = map[*Module]modRoot{}, map[*Module]bool{}
	}
	if r, ok := cs.roots[m]; ok {
		return r, true
	}
	if cs.noRoot[m] || m.Dir == "" {
		return modRoot{}, false
	}
	r, ok := c.findModRoot(m.Dir)
	if !ok {
		cs.noRoot[m] = true
		return modRoot{}, false
	}
	cs.roots[m] = r
	return r, true
}

// checkCeiling reports XO1309 when the current module calls (or takes as
// a value) fo, a function of another module that a ceiling covers, and
// fo declares an effect above the ceiling. Only the effects fo declares
// itself count: effect parameters are the caller's own effects passed
// through (callbacks), not the dependency's.
func (c *checker) checkCeiling(fo *FuncObj, sp token.Span) {
	if fo == nil || fo.Module == nil || fo.Module.IsStd || fo.Declared == nil || c.mod == nil || c.mod.IsStd || fo.Module == c.mod {
		return
	}
	var labels []string
	seen := map[string]bool{}
	for _, it := range fo.Declared.Items {
		if it.Label != "" && !seen[it.Label] {
			seen[it.Label] = true
			labels = append(labels, it.Label)
		}
	}
	if len(labels) == 0 {
		return
	}
	from, ok := c.moduleRootOf(c.mod)
	if !ok {
		return
	}
	to, ok := c.moduleRootOf(fo.Module)
	if !ok || to.dir == from.dir {
		return
	}
	cs := &c.ceil
	if cs.ceilings == nil {
		cs.ceilings = map[string]map[string][]mod.CeilingSource{}
	}
	ceils, ok := cs.ceilings[from.dir]
	if !ok {
		ceils = mod.Ceilings(from.dir)
		cs.ceilings[from.dir] = ceils
	}
	srcs := ceils[to.path]
	if len(srcs) == 0 {
		return
	}
	sort.Strings(labels)
	version := ""
	if m, ok := c.deps.depDirs[to.dir]; ok && !m.Workspace {
		version = " " + m.Version
	}
	for _, src := range srcs {
		var over []string
		for _, l := range labels {
			if !src.Ceiling.Admits(l) {
				over = append(over, l)
			}
		}
		if len(over) == 0 {
			continue
		}
		list := "`" + over[0] + "`"
		for _, l := range over[1:] {
			list += ", `" + l + "`"
		}
		where := "xo.mod"
		if src.Version == "" {
			where = "xo.work"
		}
		d := c.errorf(diag.EffectCeiling, sp, "`%s` of %s%s performs %s, above the effect ceiling `%s` of that dependency in %s", fo.DisplayName(), to.path, version, list, src.Ceiling, where)
		pos := token.Pos{Line: src.Line, Col: 1}
		note(d, token.Span{File: relDir(src.File), Start: pos, End: pos}, "the ceiling is declared here")
		note(d, token.Span{}, "review why %s needs %s; then raise the ceiling, or require a version within it (decision 0086)", to.path, list)
		return
	}
}
