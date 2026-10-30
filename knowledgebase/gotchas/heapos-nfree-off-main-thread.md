# heapos.nfree called from a worker thread segfaults the process

Status: Observed (2026-10-29, a scratch probe, xo-lang-6)

Symptom: a scratch build of xo that printed the allocator's free blocks
per size class (heapos.nfree(c), exposed as a debug function) from the
emit workers' tasks died with "SIGSEGV at address 0x1 in
xo_fiber_entry" on every program it built; the same table printed from
the driver's main thread (compiler/xo main.xo's lap) worked, and so did
debug.mem_mapped and mem_peak from the workers.

Cause (not traced to the line; the probe was scratch code): nfree reads
the class's magazine lists, which the allocator keeps per thread in
part; a worker thread's state for a class it never allocated from is
what the walk crossed. The exported debug functions (mem_mapped,
mem_peak, mem_label) read global counters and are safe anywhere.

Fix: read per-class free counts only from the main thread (XO_HEAPSTATS=1
prints them at exit; a driver lap prints them between phases). Do not
export nfree to std/debug without a thread guard.
