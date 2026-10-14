# xotool.Result.Binaries uses Go's target names (wasip1/wasm)

Status: Observed (2026-10-14), fixed (2026-10-14).

Symptom: internal/gen TestCrossCompile failed in the Linux arm64
milestone run at main f8ebc9ec with `wasi/wasm32: no binary at ` (an
empty path), while its check of four binaries passed.

Cause: not a regression of the wasm build. xo prints one `os/arch:
path` line per target in Go's names, as the Go `xo` did, so the spec's
`wasi/wasm32` (core 12.1) is printed and keyed as `wasip1/wasm`. Before
decision 0215 the test looked the binaries up by build.ParseTargets'
Targets, which converted the name; the port to internal/xotool looked
them up by the spec's string.

Fix: xotool.Target converts a target name the way xo parses it
(wasi to wasip1, wasm32 to wasm); TestCrossCompile looks binaries up
through it, and xotool's TestTarget covers the conversion.
