package rt

import (
	"runtime"
)

func runtimeGOOS() string { return runtime.GOOS }

// Testing_Expect is the value of `expect(x)`; comparisons are lowered by
// the compiler.
type Testing_Expect[T any] struct{}
