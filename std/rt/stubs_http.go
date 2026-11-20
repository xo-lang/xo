//go:build !xo_nohttp

package rt

import (
	"strings"
)

// Http_BodyWriter is the body of a streamed response (std.md 4.3).
type Http_BodyWriter struct{ w *bodyWriter }

// XoDebug implements Debug.
func (Http_BodyWriter) XoDebug(b *strings.Builder) { b.WriteString("<body writer>") }
