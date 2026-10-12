# Runtime code in Xo is slow where it touches bytes one at a time

Status: Observed (2026-10-12, decision 0180)

Symptom: the first port of the native runtime's Unicode operations to Xo
(std/native/llrt/unicode.xo, results built with
`BytesBuilder.write_byte`) took 9.4 billion instructions for 200
`to_lower` calls over 468 KB of ASCII on arm64, against 0.86 billion for
the C it replaced (11 times); mixed script text 3.7 times; even pure Xo
loops that only read bytes cost about 14 instructions a byte on arm64.

Cause: three costs that C does not pay. (1) `write_byte` and
`write_rune` are C calls per byte (`rs_push_byte`), and `UInt8(x)` or
`Rune(x)` from an Int are checked C calls (`int_conv`, `rune_of`). (2)
The arm64 backend inlines only leaf functions in one round, so small
helpers that call other Xo functions (a prelude wrapper over a runtime
module function) stay calls. (3) Its loops keep the bounds check and
recompute the data address each iteration.

Fix (in the port): hand stretches of bytes to C primitives
(`rt.bytes_span` to find the end of an ASCII run, `rt.rs_append_bytes`
to copy it, mapping ASCII letters) and keep only per rune decisions in
Xo; read tables as Bytes constants (`b[i]` is an inline load, a
`const List[Int]` is a C call per access); get UInt8 and ASCII Runes
from a 256 byte table (`BYTE[x]`, and `Rune(UInt8)` is free); add direct
tables for the common range so hot runes need no search. Result: ASCII
work faster than the C, arm64 per rune work still 1.45 to 1.8 times
the C (bench/results/runtime-pilot-2026-10-12.md). Before porting more
runtime code, measure the first version against the C with
`/usr/bin/time -l` instruction counts, not wall time.
