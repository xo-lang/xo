// Package selfhost embeds the sources of the self hosted compiler (the Xo
// module github.com/xo-lang/xo/selfhost), so the Go `xo` can build its
// driver xo1 (selfhost/xo) wherever it runs: decision 0178 switch point
// A runs `xo build|run|test` for the native backends through xo1
// (internal/selfdriver).
package selfhost

import "embed"

// Files holds xo.mod and every .xo file of the module's packages, the
// nested ones (cli/core, cli/core/format, ...) included: embed patterns
// do not recurse, so each depth is its own pattern (TestEmbedComplete).
//
//go:embed xo.mod */*.xo */*/*.xo */*/*/*.xo
var Files embed.FS
