//go:build xo_playground

package std

import "embed"

// The playground (cmd/xoplay, decision 0144) only checks programs and
// generates their Go source: the browser links them against a precompiled
// package rt. It does not embed the implementation sources, so they are
// not published inside xoplay.wasm.
var NativeFiles, Runtime, LLRuntime embed.FS
