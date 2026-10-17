# arm64: a frame over 4000 bytes or a wide value's helper failed to build

Status: Observed (2026-10-10, compiler/parser on `--backend=arm64`); fixed 2026-10-10 (decision 0146)

Symptom: `xo build --backend=arm64 compiler/parser` failed first with
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

Observed again (2026-10-10, compiler/check stage 2): `frame of
Checker.habit_call too large (29568 bytes)`, over the 28 KiB limit, on a
function of about 150 lines (a `match` of 20 habit names, each arm
calling the checker and building syntax nodes), after a field was added
to the `Checker` struct. It had built a commit earlier. Workaround used:
one small function per arm. Each spilled value still gets its own slot,
so a long function's frame grows with its length; the next step for the
backend is reusing spill slots whose values are not live at the same
time (an interval coloring over the slots in `internal/arm64`, with
`habit_call` before the split as the test case), which would also shrink
the 9 to 20 KiB frames of the parsers.

Observed a third time (2026-10-10, compiler/check stage 5): adding a
`Map[Int, Bool]` field to the `Checker` struct (to keep the read flag of
locals beside the objects) made `frame of Checker.resolve_named_type too
large (29072 bytes)`; the same commit without the field built. The flag
went back into `Obj` (`used`). A field of `Checker` grows the frame of
every long method that copies or spills it, so a change to `Checker` must
be built with `--backend=arm64` before it is committed
(`TestSelfhostChecker` builds all three backends without `-short`).

Fixed (2026-10-10, decision 0146): spilled values share a slot when
their live ranges do not overlap, which took the largest frame of the
self hosted checker from 28,176 to 6,448 bytes, and a frame past 28 KiB
no longer fails: the function is allocated again with x28 kept free and
every sp relative access past its reach goes through `add x28, sp, #hi,
lsl #12` (test program `internal/arm64/testdata/run/huge_frame`, about
36 KiB, `TestFarFrame`). The split `habit_*` functions and the `used`
flag in `Obj` stay as they are: both read well, and neither is needed
for the build any more. Building a change to `Checker` with
`--backend=arm64` before committing is no longer required for frame
size. Any new sp relative load or store form must be listed in
`spRefRE` (emit.go) so a far frame rewrites it.
