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

compiler/llvm sched.xo read the thread local table with a plain
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
evaluates on the thread that runs it (compiler/llvm sched.xo
sched_tlocal). Any new thread_local in emitted IR must do the same.

## To check when CI is re-enabled: the runner image's clang version

Observed 2026-10-16 in CI run 38954484970 (linux/arm64 runner, later
cancelled because CI is off): the internal/selfhost, internal/arm64 and
std job's links failed with 109 `undefined reference to
'llvm.threadlocal.address.p0'` from /usr/bin/ld, plus missing runtime
functions in the same objects. The intrinsic is LLVM 16 and later; the
likely cause is the runner image's clang being older than that, so the
call is left as a call to an external symbol. Docker's clang 19
(scripts/docker-linux.sh) links the same programs. When CI comes back,
check `clang --version` on the runner image (infra/ci-runners) and
raise it, or make the toolchain check reject a clang without the
intrinsic.

Known issue (Observed 2026-10-20, Xo lang 4; Linux is best effort): the
fix needs LLVM 16 or later. clang 15 has no llvm.threadlocal.address, so
it is an undefined function: CI run 39465426358 (linux/arm64, clang-15)
failed internal/arm64 TestTraceback/llvm and TestTracebackInlined/llvm at
the link (`undefined reference to llvm.threadlocal.address.p0` in
xo_tlocal_addr, xo_tlocal_get, xo_tlocal_set). It predates 2026-10-20
(compiler/llvm sched.xo, before decision 0249's move). Use clang 16 or
later for the LLVM backend; a clang 15 fallback would need the address
read made opaque another way (an inline asm `mrs` of tpidr_el0), so
that LLVM cannot treat it as a constant.
