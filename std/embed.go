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

// NativeFiles, Runtime, and LLRuntime are in embed_impl.go: the playground
// build (tag xo_playground, cmd/xoplay) leaves them empty.
