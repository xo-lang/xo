# rt.heap_large is the runtime's malloc, not only the heap's big blocks

Status: Observed (2026-10-14, work item B21)

Symptom: mapping rt.heap_large's blocks with mmap (heapos.xo's first
version) passed every golden and runtime test but crashed cmd/xo
TestAttenuationFakes: both test binaries exited with status -1.

Cause: rt.heap_large, rt.heap_large_resize, and rt.heap_release began as
alloc.xo's blocks above the size classes, but the runtime in Xo took
them up as its malloc: testworld.xo's records, pool.xo's worker and
fiber records, fault.xo's strings, stdio.xo's buffer, obj.xo's drop
worklist. The C freed some of those with free (xo_testing.c's
`free(kept - 1)` until 2026-10-15, task_free's fault strings), which a mapping is not;
and every 32 byte record became a 64 KiB mapping.

Fix: alloc.xo's big blocks have their own platform calls
(rt.heap_big*, heapos.xo); rt.heap_large stays the C library's malloc
until the runtime has an allocator of its own for its records and the
C's frees of them go through it. Before changing a platform call, grep
std/native for every caller, not only the module it was made for.
