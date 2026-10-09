package gen

import (
	"sort"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
)

// Zone data per use (std.md 13, decision 0094): the generator records
// which zones `time.zone` can be asked for, and internal/build embeds
// their data. A constant name embeds that zone; a name known only at run
// time (or `time.zone` used as a function value) embeds the whole
// database, and the build prints a note naming the call.

// TZUse is what a program needs of the zone database.
type TZUse struct {
	// Names are the constant zone names, sorted.
	Names []string
	// Dynamic lists the calls (file:line:col) whose zone name is known
	// only at run time, sorted.
	Dynamic []string
}

type tzState struct {
	names   map[string]bool
	dynamic map[string]bool
}

func isTimeZoneFn(fo *check.FuncObj) bool {
	return fo != nil && fo.Recv == nil && fo.Name == "zone" && fo.Module != nil && fo.Module.IsStd && fo.Module.Path == "std/time"
}

// noteZoneCall records a call of time.zone.
func (f *fnGen) noteZoneCall(x *ast.CallExpr, fo *check.FuncObj) {
	if !isTimeZoneFn(fo) {
		return
	}
	var arg ast.Expr
	if len(x.Args) > 0 {
		arg = x.Args[0].Value
	}
	if arg != nil {
		if name, ok := check.ConstString(f.info, arg); ok {
			f.g.tzNote(name, "")
			return
		}
	}
	f.g.tzNote("", f.g.loc(x))
}

// noteZoneValue records time.zone used as a function value: any name.
func (f *fnGen) noteZoneValue(fo *check.FuncObj, at ast.Node) {
	if isTimeZoneFn(fo) && at != nil {
		f.g.tzNote("", f.g.loc(at))
	}
}

func (g *gen) tzNote(name, dynamicAt string) {
	if g.tz.names == nil {
		g.tz.names, g.tz.dynamic = map[string]bool{}, map[string]bool{}
	}
	if dynamicAt != "" {
		g.tz.dynamic[dynamicAt] = true
		return
	}
	g.tz.names[name] = true
}

func (g *gen) tzUse() TZUse {
	var u TZUse
	for n := range g.tz.names {
		u.Names = append(u.Names, n)
	}
	for d := range g.tz.dynamic {
		u.Dynamic = append(u.Dynamic, d)
	}
	sort.Strings(u.Names)
	sort.Strings(u.Dynamic)
	return u
}
