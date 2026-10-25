# A fault position is an ABI between the emitters and the runtime

Observed 2026-10-25 (the fault location as a file, line, and column,
decision 0228's addendum).

## Symptom

Changing what a checked operation's stub passes to the runtime (the
location argument) without bumping `compiler/arm64/fcache.xo`'s
`FN_CACHE_VERSION` and `partcache.xo`'s `PART_CACHE_VERSION` reuses cached
function and part text from the previous ABI: the stale stub passes a
pointer where the new runtime reads a file, line, and column, and a fault
then reports nonsense or crashes in `fault.xo pos_text`. The asm goldens
also fail visibly, which is the cheap signal.

The reverse trap: `cabi.xo fault_raise_msg(m, loc)` cannot simply grow
arguments, because `std/native/internal.xo` hands its address to qsort as
the comparator of the fault test (two arguments: message, location).
Position raises need their own entry (`fault_raise_pos`, reached through
`rt.static_word` 90, set by `fault.init_pos`).

## Fix

- Bump both cache versions in the same change as any change to a stub's
  arguments; the emit text is not part of the function key.
- Keep a two-argument `fault_raise_msg` for the C-facing paths and add a
  separate export for a new argument shape.
- Re-record the arm64 asm goldens (`go test ./internal/arm64 -run
  '^TestGoldenAsm$' -update`) and run the fault goldens on arm64, LLVM,
  and linux/amd64 (the stderr line is the contract).
