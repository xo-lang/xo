package rt

import (
	"reflect"
	"sync/atomic"
)

// Cells (decision 0613 step 5, option B on the Go backend): an interface
// with a `var self` method holds its concrete value in a generated cell,
// `type xoCell_N struct{ V T; S uint32 }`, through a pointer, so a `var`
// method call updates the value in place. A copy of the interface value
// shares the cell; like a list's block (see List), the cell's S is a
// sticky shared flag that the generated code sets wherever it copies the
// interface value out of a place (MarkI, or MarkP through markElems), and
// a `var` call through an interface value first replaces a shared cell
// with a copy of its own (Unshare). So no interface value observes a
// write made through another, and an unshared cell is written in place.

// celler is implemented by generated cells.
type celler interface {
	XoShared() bool
	XoCopy() any
}

// CellMark sets a cell's shared flag.
func CellMark(s *uint32) { atomic.StoreUint32(s, 1) }

// CellShared reports a cell's shared flag.
func CellShared(s *uint32) bool { return atomic.LoadUint32(s) != 0 }

// MarkI marks the cell (or the lists) an interface value holds and
// returns it: the generated code wraps an interface value it copies out
// of a place.
func MarkI[I any](v I) I {
	if m, ok := any(v).(marker); ok {
		m.XoMark()
	}
	return v
}

// Unshare is the interface value at p, its cell first replaced by a copy
// of its own when the cell is shared: the receiver of a `var` method call
// through an interface value.
func Unshare[I any](p *I) I {
	if c, ok := any(*p).(celler); ok && c.XoShared() {
		*p = c.XoCopy().(I)
	}
	return *p
}

// isIface reports whether T is an interface type (no allocation, unlike
// boxing a zero T).
func isIface[T any]() bool {
	return reflect.TypeFor[T]().Kind() == reflect.Interface
}
