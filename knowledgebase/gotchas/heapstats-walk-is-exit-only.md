# XO_HEAPSTATS' free list walk is safe only at exit

Status: Observed (2026-10-31, xo-lang-2, decision 0448 stage 3 probes)

Symptom: a probe that called the llrt's heapstats (entry.xo, the
XO_HEAPSTATS report) from a heap label in the middle of xo's emit
crashed the self build with signal 11 (rc 139) when 8 emit workers were
running. The same probe at the first part's end (one worker done, the
rest starting) printed its table and exited 0, so the crash is timing
dependent.

Cause: heapstats calls heapos.nfree(c) per class, which walks every
heap record's free list and spare magazine and the central list's
magazines (heapos.xo nfree, list_len_c). It takes the heap records'
list lock (hlock) but not the owning threads' free lists: a list's
owner pops and pushes with no lock (the single threaded fast paths,
alloc.xo alloc_pop and free). A walk while the owner runs follows a link
the owner just reused and reads a block that is no longer free. At exit
(entry.xo finish) the workers are joined, so the walk is safe there,
which is the only place the shipped report runs it.

Fix: do not call heapstats or heapos.nfree while other threads
allocate. For a mid run reading, print only counters that need no walk:
heapos.chunks(), heapos.mapped_bytes() (membudget), heapos.peak_bytes(),
heapos.cache_bytes() (the probe "heapcounts" of
~/xo-lead/notes/xo-lang-2 did this). Free bytes by class mid run would
need each worker to report its own lists at a safe point (its idle path
or the end of a part), never a walk from another thread.
