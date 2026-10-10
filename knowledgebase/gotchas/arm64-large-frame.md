# arm64: a frame over 4000 bytes or a wide value's helper failed to build

Status: Observed (2026-10-10, selfhost/parser on `--backend=arm64`)

Symptom: `xo build --backend=arm64 selfhost/parser` failed first with
`arm64 backend: frame of Parser.parse_file too large (5120 bytes)`, then,
once that was allowed, in the assembler with `index must be a multiple of
8 in range [-512, 504]` on `stp x29, x30, [sp, #688]` in `// dropp
MapEntry` helpers. The LLVM backend built the same program.

Cause: the backend gives each spilled value its own slot, so a long
function with many values live across calls (a parser's `match` over
declaration kinds) gets a frame of hundreds of slots; the emitter refused
frames over 4000 bytes. Separately, the reference counting helpers of a
value of 80+ words put its stack argument words below the frame record,
so the record's offset passed the 504 byte reach of `stp`/`ldp`.

Fix (decision 0128): frames up to 28 KiB, allocated a page at a time with
a probing store (`probeSP`) so the guard page below a fiber stack is
touched rather than skipped, and `add x29, sp, #hi, lsl #12` for a frame
record past 4 KiB; helpers store the record with two `str` past 504
bytes (`recordAt`). Test programs `internal/arm64/testdata/run/big_frame`
(about 9 KiB, also on a task's fiber) and `wide_values` (80 word values in
lists, enum payloads, closures). Any other `[sp, #off]` pair instruction
added to the backend needs the same reach check.
