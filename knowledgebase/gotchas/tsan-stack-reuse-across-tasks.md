# --race reports a race between two tasks that only reused one stack

Status: Observed (2026-10-26, the owner's Windows tester on v0.32.0, windows/amd64 `--race`, WARNING: DATA RACE
every run; reproduced on darwin with `xo build --race --backend=arm64` and clang's ThreadSanitizer runtime,
and on linux/arm64 in Docker). Fixed in sanhook.xo `dead_stack_reset`.

Symptom: tasks that loop only over `m.with(fn(var c: Map[Int, Int]) { c[k] = v })` (also `Mutex[Int]` with
`c += 1`, List pushes) report a data race on every run although the mutex orders every access. Two tasks, no
loop, are enough: `scope s { for t in 0..2 { s.spawn(fn() { m.with(fn(var c: Int) { c += t + 1 }) }) } }`.
One ordinary call in the loop, or a `sleep` before the `with`, makes it clean.

Cause: a spawn site whose closure takes a Mutex in a loop starts "small" (decision 0363), so its burst runs one
task after another at the spawner's join, on one fiber stack. Each task is a ThreadSanitizer context of its own
(`task_new` at the spawn), and nothing orders task 2 after task 1. The write through the `var` parameter's
pointer is instrumented (the arm64 backend skips only sp and x29 relative accesses), and the slot in the
caller's frame has the same address in every task, so the shadow memory of the finished task's write is still
there and the next task's write at that address is "concurrent" with it. Tasks that run on workers have
different stacks, which is why v0.27.0 (and any program with one more call in the loop) was clean; a cached
fiber stack reused by a later task could always do it.

Fix: when a task starts (`sanhook.task_enter`), in its own context, the shadow of up to 64 KiB of the dead
stack below the running frame is reset: the C flavor with `__tsan_java_free` then `__tsan_java_alloc` (which
resets a range like a malloc, without a write of the caller's; the Java interface is initialized once over the
application range), the Go flavor (windows) with `race_alloc` (racemalloc). A remap of the pages was tried
first and rejected (it zeroes live frames sharing a page, and TSan's mmap interceptor imitates a write by the
caller, which races with the next task's).

Avoid: do not treat memory a finished context touched as free of history when a new context reuses it
(stacks, arenas, fiber caches); a reuse needs the equivalent of a free and an alloc in the new context.
Tests: internal/llvm TestRaceSyncTable and TestRaceSyncTableArm64 (race_sync case mutex_loop, which failed on
the arm64 backend before the fix), scripts/ci/windows-race.sh (the same case on windows/amd64).
