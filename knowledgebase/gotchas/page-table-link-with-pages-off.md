# A page table link read with page blocks off crashes a --race build

Observed 2026-10-30 (internal/llvm TestRuntimeAlloc at 4c397e82b, decision 0425's
first main check).

## Symptom

Under ThreadSanitizer (`--race`, the internal/llvm TestRuntime* tests) a program
with threads dies in the sanitizer's own code:

```
ThreadSanitizer: SEGV on unknown address 0xffffaababa802750 ... in OUTLINED_FUNCTION_7
```

with registers holding pointers whose high bytes are 0x5d5d5d5d5d. The same
program is fine without --race, and every llrt test and the fixed point pass.

## Cause

Decision 0425 keeps the prefixed layout in sanitizer and --race builds
(heapos.pages_on false, the heap record's PAGES word 0): their big blocks come
from the C library and can be page aligned themselves. Every link read of a
magazine or free block must then pick the layout by that flag (alloc.xo
cnext, lnext with their `pg` argument). One loop did not: free_mt's count of
the central magazines before a scavenge (classes from SCAVENGE_FROM, 69) read
`pword(w, PT_CLINK)`, the page table word of the 4 MiB base of a block that is a
malloc'd prefixed block under the sanitizer, so the link came from unrelated
memory and ThreadSanitizer faulted computing its shadow.

## Fix

`w = cnext(pg, w)` in that loop. The regression check is
`go test ./internal/llvm -run '^TestRuntimeAlloc$' -count=1`, which fails every
run at 4c397e82b and passes at its parent. Rule (agent-tools.md, gates): a change
to std/native/llrt's allocator, scheduler or sync code runs
`go test ./internal/llvm -run 'TestRuntime|TestRace' -count=1` in its gate; the
llvm step of gate.sh is not skipped for std/native/llrt files, but the bare
minimum gate of 2026-10-21 names it only when the agent does.
