# Each fiber's stack held 2 MiB of resident memory with transparent huge pages "always"

Status: Observed (2026-10-22)

## Symptom

The CRUD server (bench/perf/http/xo) at 1024 connections had a peak RSS of
2.1 to 2.3 GB against Go's 66 to 78 MB. RSS grew linearly with the
connections (64: 158 MB, 256: 561 MB, 1024: 2.2 GB, about 2.2 MB each)
while the allocator's chunks stayed at 28 MB (XO_HEAPSTATS=1).

## Cause

Each fiber has a 64 MiB reserved stack (map_stack, MAP_NORESERVE). On a
Linux whose `/sys/kernel/mm/transparent_hugepage/enabled` is `always`
(Docker Desktop's VM, and the default of several distributions), the first
touch of the stack's top page faults in a whole 2 MiB huge page. Every
live fiber's mapping had Rss 2052 KB in /proc/PID/smaps, for a few KiB of
frames. Go's goroutine stacks are small and do not reach the huge page
size.

## Fix

map_stack calls madvise(MADV_NOHUGEPAGE) on the mapping (std/native/llrt
pool.xo). At 1024 connections the peak RSS is 47 to 57 MB (Go: 66 to 78
MB); throughput did not change within the run to run noise.

The --race path (sanhook.stack_map, the C library maps the stack under
ThreadSanitizer) needs no madvise (measured 2026-10-22, linux/arm64 Docker,
THP always, --race CRUD server at 256 connections, with and without the
call: 255 and 263 MB RSS, the same within noise). Its stacks are mappings
of about 4 MiB, not 64 MiB, each 8 KB resident (/proc/PID/smaps), and the
RSS is ThreadSanitizer's shadow memory (mappings of 1792, 1024 and 256
KB).

Not covered, on purpose: worker thread stacks. libc's pthread maps them (a
few per process, THREAD_STACK reserved), so at most 2 MiB resident each,
fixed and not per connection; covering them would mean mapping the stack
ourselves and passing pthread_attr_setstack.

## How to avoid

A mapping that is touched sparsely and held for long (stacks, rings,
arenas) gets MADV_NOHUGEPAGE, or is sized below the huge page. Check
`grep -E '^(Size|Rss)' /proc/PID/smaps` for mappings whose Rss is 2048 KB.
