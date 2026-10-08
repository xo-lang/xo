// Package std embeds the xo standard library declaration files (std/*.xo)
// and the Go source of the runtime package (std/rt) that implements them.
// The declarations are bodiless signature listings of spec/std.md, parsed
// in parser.SignatureMode and loaded by internal/check. The encoding of
// spec/std.md pseudo syntax is documented at the top of prelude.xo. The
// runtime is copied into every build directory by internal/build.
package std

import "embed"

// Files holds every std/*.xo file. prelude.xo is the prelude; each other
// file `name.xo` is the module `std/name`.
//
//go:embed *.xo
var Files embed.FS

// Runtime holds the Go source of package rt (std/rt/*.go).
//
//go:embed rt/*.go
var Runtime embed.FS

// LLRuntime holds the C runtime of the P3 LLVM backend prototype
// (std/llrt/xo_rt.c), compiled by clang with the IR internal/llvm emits.
//
//go:embed llrt/*.c
var LLRuntime embed.FS
