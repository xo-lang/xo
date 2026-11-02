# A freed big list is reused only when the next one fits

Status: Observed (2026-11-01, xo-lang-2).

Symptom: several lists of the same data, each freed before the next is allocated, still touch
new pages for each (`vmmap` shows several resident mappings of about the same size). Sizing the
lists exactly did not lower RSS.

Cause: the large cache takes a cached mapping only when its size is from `need` to 4 times
`need`. A list sized exactly is a little too small for a successor with a few more elements. A
copy still held elsewhere also keeps a block alive: `let fresh = c` for a retry held a whole
Checker's records to the end of the check.

Fix: give the earlier lists room (`with_capacity(n * 3 / 2)`); untouched pages cost address space
only. Find holders by reading what is copied before the free, then confirm with per mapping sizes
from `vmmap` at each phase and `go tool pprof -sample_index=alloc_space -traces`. Decision 0448
case (c) took coll from five such lists to two.
