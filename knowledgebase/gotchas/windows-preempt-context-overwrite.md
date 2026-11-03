# Windows preemption: a second kick overwrote the saved CONTEXT

Status: Observed (2026-11-02, Xo Lang 1)

Symptom: on windows/amd64 under load (32 copies beside CPU burners),
async_preempt_regs hung (26 of 128 runs past 300 s; the median run is 5 s),
exited with C0000005 and an empty stderr (13 of 128), or printed a wrong
integer sum. It passed 80 of 80 runs on an idle machine.

Cause: pool.xo win_kick suspended the worker thread and called
GetThreadContext straight into the fiber's save area (win_ctx), then
redirected the thread to on_wpark, which yields and later restores that
CONTEXT. A suspended, redirected thread may not run for a long time on a
loaded machine; the monitor's next kick read its (redirected) context into
the same save area before on_wpark had run, so the restore resumed on_wpark
itself (a loop), a half-run prologue (a fault), or registers that were not
the interrupted code's. Linux and darwin cannot hit it: the kernel saves each
signal's context on the stack.

Fix: read into the scratch half of the save area and copy into win_ctx only
when the kick redirects the thread (Go's preemptM likewise reads into a
context of its own). A runtime record a later kick can reach must not be
written until the earlier use of it is known to be over.

How to check: run the async_preempt goldens in 32 copies at once beside a
CPU burner per core on the Windows host, under a timeout of a minute or
two; record each run's NT exit code (through PowerShell's Start-Process,
since MSYS bash folds C0000005 into 139) and keep the hangs apart from the
faults.

Still open (2026-11-02): with the fix, 8 of 256 async_preempt_regs runs under
the same load print a wrong integer sum (no hang, no fault). Preemption causes
it (0 of 128 with XO_PREEMPT=0); it is not an early loop exit (the wrong sums
match no iteration count of the loop); the loop's integers live partly in
stack slots above rsp and its floats in xmm registers, and only the integer
sum is ever wrong. An async-only build (XO_PREEMPT_POLLS=off) had 0 of 128,
which may be chance at that rate.
