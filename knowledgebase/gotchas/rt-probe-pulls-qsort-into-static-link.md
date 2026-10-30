# The compiler must not call `internal.rt_probe`: a static Linux link fails on qsort

Observed 2026-10-30 (decision 0435's first build on the linux/amd64 offload host).

## Symptom

The self build (`xo build compiler/xo`) on Linux fails at the link:

```
xo: link: qsort is not defined (a static link with no C library)
```

while the same tree builds and runs on macOS, where the link has libc. The
fixed point on the Mac and every Mac gate pass.

## Cause

`std/internal.rt_probe` is one function with every probe op in one `match`;
its `"qsort_fault"` arm calls `sys_c.qsort` (the test of a fault raised
inside a C library callback, decision 0241). Any program that calls
rt_probe keeps the whole match, so a static link without a C library
(the native Linux targets) has an undefined `qsort`. The compiler's first
call of rt_probe (the arena toggle around each parse) made xo itself such
a program.

## Fix

Give the compiler its own std/internal function (`internal.arena_mode`)
whose native body calls the runtime directly, and keep rt_probe for tests
and benches. In general: a std/internal function the compiler, a std
module or a program may call must not share a body with a probe that names
a C library symbol; test only probes stay in rt_probe. A gate for a change
that adds a std/internal call to compiler/ builds xo on a Linux host
(`~/xo-lead/agents/amd64-run.sh REV NAME sample` builds it natively) before
the push: the Mac cannot see this failure.
