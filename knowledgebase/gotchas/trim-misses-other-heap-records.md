# A heap trim walks only one record, and maybe not the allocator's

Status: Observed (2026-11-04, Xo Lang 2, building decision 0505).

## Symptom

`debug.mem_trim()` gives back nothing for freed blocks of 16 KiB to 256
KiB: the resident set stays where it was (324 to 332 MB of 160 MB of freed
16 and 64 KiB blocks on linux/arm64), even with the Linux advice in place
and every `madvise` returning 0.

## Cause

Two, both about which heap record the trim walks:

1. A heap record's free lists belong to one thread, and a task's frees
   land on whichever thread ran it. `heapos.trim_thread` walked only the
   calling thread's record, so blocks freed by tasks on other workers were
   never seen.
2. Before a program's first spawn, the allocator uses the single threaded
   record (obj.xo's heap word, static word HEAP), which no thread's
   TLS_HEAP names. `trim_thread` called `thread_heap()`, which found no
   TLS record and made a new, empty one, and trimmed that.

## Fix

`trim_thread` takes the allocator's record (the single threaded one while
set, else TLS_HEAP; nothing when neither), and `mem_trim` moves the
pool's trim epoch so each worker trims its own record at the top of its
loop (decision 0505's addendum). To see which record holds a program's
free blocks: XO_HEAPSTATS=1 lists the free blocks per class; a temporary
print of each record in the heaps list (heapos.xo heaps_head, H_NEXT)
with its class lists' heads shows where they sit. Do not print from
`thread_heap` itself: the print allocates and recurses into it (the build
segfaulted).
