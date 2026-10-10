# Any binary that imports internal/build embeds the runtime source

Status: Observed (2026-10-10)

Symptom: the first `xoplay.wasm` contained the full source of std/rt,
test files included (`t.Errorf(... /users/204 ...)` in `strings`), though
the playground never writes a build directory.

Cause: `std.Runtime` (`//go:embed rt/*.go rt/*.s`) is referenced by
`internal/build` (buildInfo hashes it, writeTree copies it), so the linker
keeps the embedded files in every binary that links build, used or not.
The same holds for `std.NativeFiles` and `std.LLRuntime`.

Fix: build anything published before the source is public with
`-tags xo_playground` (`std/embed_playground.go` leaves the three empty), and
scan the result: `scripts/build-playground.py` output was checked for local
paths and test source with a regexp over the decompressed assets.
