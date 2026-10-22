# ThreadSanitizer's Go flavor on Windows: four traps of the first Wine runs

Status: Observed (2026-10-22, Xo Lang 2, decision 0273's Windows
addendum; windows/amd64 --race under scripts/docker-wine.sh box64,
XO_THREADS=1).

1. Symptom: every gorace check folded to false in a --race build (heapos.xo
   map took the plain VirtualAlloc arm, so the heap was outside the
   shadowed arena and the first instrumented load of a list faulted in
   __tsan_read). Cause: compiler/ssa call.xo sym_present folds every
   `__tsan_` and `__sanitizer_` symbol to false on Windows (no C flavor
   runtime links there; the fold drops the C flavor's pthread paths).
   Fix: `__tsan_go_start` folds to the build's --race flag instead
   (Checker.race), so sanhook.gorace() is a constant on Windows.
2. Symptom: `race: limit on 0 simultaneously alive goroutines is
   exceeded, dying`. Cause: a ThreadState made (__tsan_go_start) before
   __tsan_init: rt.heap_large allocates before heapos.xo main_record, and
   the race heap reset the block's shadow with the current context. Fix:
   sanhook.race_init runs once at the first use (race_arena, race_thr).
   Also: the Go flavor has no thread's own context (the C flavor's
   current fiber), so a thread that runs code before its first switch
   hook (main's fiber starts on a thread of its own) had thr 0 and
   __tsan_read faulted on it; race_thr makes one for such a thread.
3. Symptom: `CHECK failed: tsan_rtl.cpp:661 "((meta_end)) >
   ((mapped_meta_end))"`. Cause: the Go flavor's MapShadow takes its first
   call for the data and bss and later calls for one heap growing upward:
   each call's meta shadow (half the size, rounded to 64 KiB) must end
   above the last. Two 64 KiB arena pieces share one 64 KiB meta unit.
   Fix: the arena's granule is 128 KiB (sanhook.xo ARENA_GRAIN), the image
   is mapped first (race_init), and the arena only grows upward under its
   lock. Everything instrumented code touches must be in the image or the
   arena (heap mappings, the race heap, fiber stacks); app memory must lie
   below 0xe0_0000_0000, so the PE has no ASLR (pelink fixed_base).
4. Symptom: a false race between a finished task's write and main's read
   after its await, while the planted race was found. Cause: tasks.xo
   tells ThreadSanitizer about task completion only when the services
   record's SV_TSAN is set, which ctx.xo set from sanhook.tsan() (the C
   flavor) alone. Fix: tsan() || gorace(). Grep for sanhook.tsan() when
   adding a flavor: each use is either a C flavor path (C library calls)
   or means "any race detector".

5. Symptom: a program that grew a Map and then spawned died under Wine
   with exit 5 (an access violation in cabi.heap_threads, whose recount of
   the single threaded heap's free lists met a link of 0x50). Cause: in
   the race heap every block is big, so a small one's prefix is minus a
   small size, which cabi.xo heap_big_release took for a heap profile
   sampled block (minus its class) and pushed on that class's free list:
   race heap memory in the program heap. The C flavor's user_mode() was
   exempted; gorace() was not. Fix: exempt gorace() too; race_sync's
   map_grow case runs it. A non-race build never has such a prefix (its
   big blocks are past MID_MAX). Found when file:line's DWARF loading
   (map inserts at the first access) crashed every race program.

Also: the Go flavor's __tsan_release is a store (ReleaseStore), the C
flavor's a merge (Release); the runtime's release goes through
race-windows.cpp's xo_tsan_go_release, which calls __tsan_release_merge.
Box64's Wine sometimes loses a run's output (an empty last line); rerun
before reading a blank as a failure.
