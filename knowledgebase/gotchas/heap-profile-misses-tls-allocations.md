# XO_PROF=heap misses the TLS per-connection allocations

Status: Anticipated (2026-10-31; cause not yet known)

Symptom: an idle server with 40,000 TLS connections holds about 750 MB
(18.8 KB a connection), but `XO_PROF=heap` (also at rate 1) reports only
about 88 KB in use, and the profile table filled at rate 1. The profile
cannot say where the memory is.

Suspected cause: the sampler hooks the generic allocation path while the
TLS engine allocates through raw buffers (`raw.bytes_buf`, `rs_with_cap`)
and runtime blocks (`rt.heap_large`, `obj.alloc_obj`) that bypass it, or
the sampled stacks of a parked connection are dropped when its fiber
ends. Not confirmed.

Fix to try: have the heap report count every allocation entry point, and
make a full table evict rather than stop. Until then use RssAnon deltas
between experiments (bench/perf/idleconn, TestIdleConnections40kTLS) and
the XO_HEAPSTATS size class report at exit (gotchas/heapstats-walk-is-exit-only.md).

Update 2026-11-01: the sample table (65,536) fills during the handshakes (about 880 allocations per
TLS connection against 30 plain): at rate 1 with 200 connections 110,700 samples were lost, and
at rate 1024 (17,371 samples, none lost) the profile still showed only 17 objects in use at
os.proc.exit while the live block count was 74,140 for 2,000 held connections. So the in-use
list does not hold the connections' allocations even when the table has room; cause still open
(suspect: blocks allocated on a connection fiber and freed or handed over through a path the
profiler's address table does not see). Use XO_HEAPSTATS `live blocks` and the plain-versus-TLS
difference of the free lists until it is fixed.
