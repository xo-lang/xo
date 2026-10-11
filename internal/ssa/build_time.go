package ssa

import (
	"fmt"
	"sort"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
)

// Zone data per use on the native backends (std.md 13, decision 0094), as
// internal/gen/timezone.go records it for the Go backend: a call of
// `time.zone` with a constant name embeds that zone, a name known only at
// run time (or `time.zone` as a function value) the whole database.
// internal/build writes the data the module asks for (Module.TZ) into
// the C runtime's xo_tzdata.c.

// TZUse is what a program needs of the zone database.
type TZUse struct {
	// Names are the constant zone names, sorted.
	Names []string
	// Dynamic lists the calls (file:line:col) whose zone name is known
	// only at run time, sorted.
	Dynamic []string
}

func isTimeZoneFn(fo *check.FuncObj) bool {
	return fo != nil && fo.Recv == nil && fo.Name == "zone" && fo.Module != nil && fo.Module.IsStd && fo.Module.Path == "std/time"
}

// noteZoneCall records a call of time.zone.
func (b *builder) noteZoneCall(x *ast.CallExpr, fo *check.FuncObj) {
	if !isTimeZoneFn(fo) {
		return
	}
	if len(x.Args) > 0 && x.Args[0].Value != nil {
		if name, ok := check.ConstString(b.info, x.Args[0].Value); ok {
			b.tzNote(name, "")
			return
		}
	}
	b.tzNote("", b.posCol(x))
}

// noteZoneValue records time.zone used as a function value: any name.
func (b *builder) noteZoneValue(fo *check.FuncObj, at ast.Node) {
	if isTimeZoneFn(fo) && at != nil {
		b.tzNote("", b.posCol(at))
	}
}

func (b *builder) posCol(n ast.Node) string {
	sp := n.Span()
	return fmt.Sprintf("%s:%d:%d", b.file(sp.File), sp.Start.Line, sp.Start.Col)
}

func (b *builder) tzNote(name, dynamicAt string) {
	if b.tzNames == nil {
		b.tzNames, b.tzDynamic = map[string]bool{}, map[string]bool{}
	}
	if dynamicAt != "" {
		b.tzDynamic[dynamicAt] = true
		return
	}
	b.tzNames[name] = true
}

func (b *builder) tzUse() TZUse {
	var u TZUse
	for n := range b.tzNames {
		u.Names = append(u.Names, n)
	}
	for d := range b.tzDynamic {
		u.Dynamic = append(u.Dynamic, d)
	}
	sort.Strings(u.Names)
	sort.Strings(u.Dynamic)
	return u
}
