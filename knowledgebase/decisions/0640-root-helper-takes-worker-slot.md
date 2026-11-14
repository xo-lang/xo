# 0640: a root thread that helps run tasks takes one of the pool's worker slots

Status: Proposed (Xo Lang 2, 2026-11-14; the lead chose option (a) the same day)

Stub: being written. Context: decision 0638 step 2's p99 measurement under a 2 CPU cgroup quota found N workers run N+1 threads of Xo work (main's root thread helps, decision 0077), so the quota throttles.
