# What stays resident after the heap's idle sweep is stacks and fragments

Status: Observed (2026-11-22, Xo Lang 1, task #673, linux/arm64).

Symptom: a variant of internal/llvm testdata/rt/heapidlelists in which
the sixteen tasks of a round hold their 3.8 MiB of small blocks until
all have made theirs (sleeping in 1 ms steps) left 9 to 13 MB resident
after three idle periods on one host, past TestRuntimeHeapIdleLists'
8 MiB, where the plain program leaves 3 to 6 MB. It read as the idle
sweep missing blocks freed by tasks that had parked.

Cause: none in the sweep. Measured on a quiet host (five alternating
95 s runs each, ~/xo-lead/notes/xo-lang-1/p673/sm3.out): the plain
program left 2.8 to 6.0 MB over its start (peaks 69 to 111 MB), the
holding one 6.2 to 9.1 MB (peaks 106 to 127 MB): about 3 MB more, with
a higher peak. /proc/PID/smaps after the idle (sm2.out) shows where it
is, plain against holding: whole 4 MiB heap chunks 64 KiB and 68 KiB
(the heap gives its chunks back); mappings of 64 KiB to 2 MiB, which
are fiber and thread stacks, 3.5 MB and 4.7 MB (66 and 77 of them);
chunks cut by partial releases 3.2 MB and 1.8 MB; small mappings 0.7
MB each. A task that parks keeps its fiber, so sixteen at once need
sixteen stacks, and the runtime keeps up to 64 stacks cached with the
pages they touched (pool.xo FIBER_KEEP; only the stacks past 64 are
unmapped by an idle worker, fiber_trim). The same program's residue
also varies by megabytes from run to run (the spinning variant left
3.7 MB once).

Fix: none needed; recorded as expected. A possible saving, not built:
an idle worker could give back the pages of the cached stacks it keeps
(about 100 KiB each here).

How to avoid: a resident set that does not come back to its start is
not a heap leak until /proc/PID/smaps says which mappings hold it: a
4 MiB chunk still resident is the heap, a 64 KiB to 2 MiB mapping is a
stack. And TestRuntimeHeapIdleLists' 8 MiB bound has little room: the
plain program left up to 6.0 MB in five runs.
