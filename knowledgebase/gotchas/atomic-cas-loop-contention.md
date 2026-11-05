# A compare and swap loop under contention burns CPU that one fetch and add does not

Observed 2026-11-05.

Symptom: micro13 spawn100k (100,000 tasks adding to one `Atomic[Int]`)
matched Go's wall time on arm64 but used 1.5 to 2.1 times its CPU; xo's
profiler put 24% flat in an `__aarch64_*` helper with no caller.

Cause: llrt atomic.xo's `add` was load, compute, compare and swap, retry
(for its overflow fault). With twelve workers on one counter, all but one
swap fail each round and retry: 84.6% of CPU samples were
`__aarch64_cas8_acq_rel` from the task body. Go's Add is one LDADDAL. On
x86 `lock cmpxchg` loops cost about as much as `lock xadd`, so amd64 did not
show it. xo's profiler cannot attribute samples in the outline helpers
(frameless, no unwind entry), so the cost looked like generic runtime work.

Fix: one fetch and add where overflow is provably impossible (operand under
2^20, value within 2^62 of zero), the checked loop otherwise
(bench/results/atomic-add-ldadd-2026-11-05.md). To find a helper's callers
on Linux: `perf record -g --user-regs=lr,sp,pc` then
`perf report --no-children -S __aarch64_cas8_acq_rel -g caller`. Any other
CAS retry loop on a shared word (atomic max, runtime counters) has the same
cost under contention.
