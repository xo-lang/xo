# The heap profile missed grown big blocks

Status: Observed (2026-10-31, ADR 0448 stage 3)

Symptom: `XO_PROF=heap` with `XO_PROF_HEAP_AT` on the self build of xo
showed 2.05 to 2.23 GB in use halfway through emit, while the heap's
counter (`--phase-times`) read 5.7 to 7.1 GB at the same moment and max
RSS agreed with the counter. The 4 MiB chunks (`debug.mem_chunks`, a
probe) explained only 3.4 GB of it, so about 3.4 GB of big blocks were
live and missing from the profile.

Cause: heapprof recorded a big block's size at its allocation only.
cabi.xo heap_big_resize told the profile about a move (moved kept the
old size) and nothing about an in-place resize, and the bytes a resize
added never reached the sampler's allocated count. A list or text that
grows by doubling (value tables, a part's assembly text) is one big
allocation followed by resizes, so it counted as its first size, or not
at all.

Fix: heapprof.resized (decision 0448 stage 3): a sampled block takes its
new size and address; an unsampled one's growth counts as allocated
bytes and can be sampled. internal/llvm TestProfHeapGrown holds a list
grown to 32 MB and checks the profile's in use.

How to avoid: before reading a heap profile's in use as the answer,
compare it with the heap's own counter (`--phase-times`, or
`debug.mem_mapped()` at the same moment) and RSS; a gap of more than the
chunks' free lists means the profile misses a path.
