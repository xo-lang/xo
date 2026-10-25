# A List of a sized integer (or Float32, Rune) takes 8 bytes an element on the native and LLVM backends

Status: Observed (2026-10-24)

Symptom: the Str sort kept its index lists as `List[UInt32]` to halve the
merge phase's index memory (commit 6bb0509e4 in a scratch worktree, never pushed). `--ab` max RSS
was 1.001 of the `List[Int]` version (21424 against 21440 KB), and a probe of
10,000,000 `UInt32` against 10,000,000 `Int` elements printed the same max
resident set size, 82,706,432 bytes, for both.

Cause: a list's element size is its element's word count times 8, except
`Bool` (1 byte). On the native backends (arm64, and amd64 which shares it)
`G.esize` in compiler/arm64/gen.xo says so, and rt.list_esize reads it
(calls.xo). The LLVM backend sizes an `Int`, `Float`, `List`, `Box`
or `Handle` at 8 (Gen.layout, compiler/llvm/gen.xo), and the SSA type has
one `Int` kind for every integer width, so `Int8`..`UInt32`, `Float32` and
`Rune` are all 8. The Go backend maps them to Go's own `int8`..`uint32`, so
its `[]uint32` is 4 bytes. `Bytes` is its own packed type (1 byte a byte).
The spec had no statement of this (core.md 2, now a note).

Fix: none yet. Use `Bytes` for bytes and plan a list of N sized ints as 8N
bytes. ADR 0369 (Proposed) costs a packed representation. A list of structs
pays the same: a struct of an `Int32` and an `Int32` is two words.
