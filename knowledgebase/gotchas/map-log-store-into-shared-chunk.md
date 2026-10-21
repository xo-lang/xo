# A log Map's new entry stored into a shared chunk stays thread local

Status: Observed (2026-10-21, Xo Lang 3; fixed the same day)

Symptom: the CRUD benchmark server built with XO_MAP_LOG=1 (decision
0291's hybrid Map) crashed with SIGSEGV within a second of concurrent
load (loadgen -c 8 and up; -c 1 ran clean), in alloc.alloc_pop: a broken
free list. Serial goldens, callgrind runs, and the fixed point never saw
it. A `--race` build reported a data race between `xo.mlog.each` (the
mark walk) and `xo.drop.User` on another thread.

Cause: a value that crosses threads (a Mutex's state when `with` ends) is
marked shared by a walk that turns thread local counts negative and stops
at objects already shared (rc.mark returns 0 for them). The log Map
appends entries in place into its chunks (`log.push`; and its put fast
path overwrites one, `log.set`), and new chunks into its directory,
without claiming them: the chunks are shared between Map values by
design. After the first walk the chunks and the directory are shared, so
an entry stored there later was never reached by a walk: its strings kept
thread local counts while readers on other threads copied them out (dup)
and dropped them with plain stores.

Fix: compiler/ssa/maplog.xo mlog_share_into marks the stored value shared
first when the list it goes into has a negative (shared) count, at every
log.push and log.set. Test: internal/llvm TestTSanSharedMaps
(testdata/tsan_maps, both Map forms under --race; 93 reports before the
fix).

Lesson: any representation that writes into a block without claiming it
(rc.claim makes a shared block thread local again, so the next walk
reaches what was written) must mark what it stores when the block is
shared, and needs a concurrent --race test, not only serial goldens.
