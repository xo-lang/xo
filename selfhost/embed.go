// Package selfhost embeds the sources of the self hosted compiler (the Xo
// module github.com/xo-lang/xo/selfhost), so the Go `xo` can build its
// driver xo1 (selfhost/xo) wherever it runs: decision 0178 switch point
// A runs `xo build|run|test` for the native backends through xo1
// (internal/selfdriver).
package selfhost

import "embed"

// Files holds xo.mod and every .xo file of the module's packages.
//
//go:embed xo.mod */*.xo
var Files embed.FS
