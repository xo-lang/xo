# LLVM merges thread local address calls across a fiber switch

Status: Observed (2026-10-13, decision 0212)

Symptom: TestGoldenThreads on EC2 Linux amd64 (clock_timers, four
threads under ThreadSanitizer) died with `SEGV ... in xo_x_fiber_park`,
a read, under `park_wait_n`; the same tests passed on the Mac.

Cause: sched.xo's fiber_park finds the thread local table with
`rt.tlocal_addr()` before the switch and again after it (the fiber may
go on on another thread). The LLVM emitters defined `xo_tlocal_addr` as a
noinline function returning a thread local's address, which LLVM
treats as a constant within a thread, so function attribute inference
made it `readnone` and the optimizer merged the two calls across the
opaque `xo_stack_switch` call: after a migration the fiber used the old
thread's table. Confirmed on the IR: without the fix fiber_park keeps
one `xo_tlocal_addr` call at -O2, with it two. Whether it shows depends
on timing (a migration while the hooks run).

Fix: `xo_tlocal_get`, `xo_tlocal_set`, and `xo_tlocal_addr` start with
`call void asm sideeffect "", "~{memory}"()` (internal/llvm sched.go,
selfhost/llvm sched.xo), so none is inferred readnone or readonly. Any
function that yields a thread local's address to code that can switch
stacks must have a side effect LLVM cannot see through; `noinline`
alone is not enough. The arm64 emitters' calls are opaque assembly and
were not affected.
