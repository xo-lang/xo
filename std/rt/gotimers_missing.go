//go:build xo_gotimers_missing

package rt

import _ "unsafe" // go:linkname

// A runtime symbol no Go release has: internal/build's
// TestGoTimersLinkFallback builds with this tag to check that a failed
// link of the Go timer view falls back to a build without it (decision
// 0073).

//go:linkname xoMissingRuntimeSymbol runtime.xoMissingRuntimeSymbol
func xoMissingRuntimeSymbol() int64

var xoMissingRef = xoMissingRuntimeSymbol

func init() {
	if xoMissingRef == nil {
		panic("unreachable")
	}
}
