package check

import (
	"fmt"
	"strconv"
)

// CurrentEdition is the newest edition this toolchain knows (core 13). A
// module whose xo.mod has no `edition` line is on this edition.
const CurrentEdition = 2026

// Editions lists every released edition, oldest first. The compiler supports
// all of them (core 13). Tests may append a future edition temporarily.
var Editions = []int{2026}

// parseEdition validates the argument of an `edition` line in xo.mod.
func parseEdition(s string) (int, error) {
	y, err := strconv.Atoi(s)
	if err != nil || len(s) != 4 {
		return 0, fmt.Errorf("edition %q is not a year (want edition YYYY)", s)
	}
	for _, e := range Editions {
		if e == y {
			return y, nil
		}
	}
	return 0, fmt.Errorf("edition %d is not supported by this toolchain (known editions: %v)", y, Editions)
}

// SupportedEdition reports whether y is a released edition.
func SupportedEdition(y int) bool {
	for _, e := range Editions {
		if e == y {
			return true
		}
	}
	return false
}

// AtLeast reports whether module m is on edition y or later. A rule that
// changes at an edition boundary tests it so code on older editions keeps
// its meaning (core 13). Std and Go binding modules are always current.
func (m *Module) AtLeast(y int) bool {
	e := m.Edition
	if e == 0 {
		e = CurrentEdition
	}
	return e >= y
}

// editionRule is a check that applies to modules on edition Since or later.
// The list is the hook through which an edition changes checker behavior;
// it is empty until an edition after 2026 changes a rule, and tests add
// entries to exercise the mechanism.
type editionRule struct {
	Since int
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
