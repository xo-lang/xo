package rt

import (
	"runtime"
	"strings"
)

func runtimeGOOS() string { return runtime.GOOS }

// Http_BodyWriter is the body of a streamed response (std.md 4.3).
type Http_BodyWriter struct{ w *bodyWriter }

// XoDebug implements Debug.
func (Http_BodyWriter) XoDebug(b *strings.Builder) { b.WriteString("<body writer>") }

// Testing_Expect is the value of `expect(x)`; comparisons are lowered by
// the compiler.
type Testing_Expect[T any] struct{}
