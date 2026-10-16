# LLVM: a thread_local global's address is a constant unless read through llvm.threadlocal.address

Status: Observed (2026-10-15, xo-fa, the goldens in Docker on
linux/arm64: 19 of 178 LLVM builds crashed; every Mac build passed).

## Symptom

LLVM-backend programs that park tasks (scopes, channels, net, http,
timers) crash on linux/arm64 with SIGSEGV or SIGBUS, randomly, even
with XO_THREADS=1: a return to an address in data (x30 == pc), or the
"every task is blocked" deadlock fault. A core dump shows a worker
running on another worker thread's native stack. Under gdb the crash
went away (other timing). The arm64 backend and darwin/arm64 were fine.

## Cause

selfhost/llvm sched.xo read the thread local table with a plain
`getelementptr ... @xo_tlocal`. For LLVM 15 and later a use of a
thread_local global is a constant address; interprocedural constant
propagation folded xo_tlocal_addr to `ret` and its callers computed
the address themselves. On Linux that is `mrs xN, tpidr_el0`, which
x.sched's fiber_main read once into a callee saved register and used
after the task had parked and gone on on another worker: the fiber
switched to the first thread's scheduler stack. On Darwin a TLS access
is a call of the variable's descriptor, which happened to be repeated.

## Fix

xo_tlocal_at takes the address with
`call ptr @llvm.threadlocal.address.p0(ptr @xo_tlocal)`, which LLVM
evaluates on the thread that runs it (selfhost/llvm sched.xo
sched_tlocal). Any new thread_local in emitted IR must do the same.
