//go:build !xo_playground

package std

import "embed"

// NativeFiles holds the implementation modules of the native backends
// (native/<name>.xo implements std/<name>, native/prelude.xo the prelude)
// and the declarations of their intrinsics (native/rt.xo), decision 0074.
// internal/check loads them only for the native backends.
//
//go:embed native/*.xo
var NativeFiles embed.FS

// Runtime holds the source of package rt: std/rt/*.go and the assembly
// of curG (std/rt/*.s).
//
//go:embed rt/*.go rt/*.s
var Runtime embed.FS

// LLRuntime holds the C runtime of the P3 LLVM backend prototype
// (std/llrt/xo_rt.c), compiled by clang with the IR internal/llvm emits.
//
//go:embed llrt/*.c
var LLRuntime embed.FS
