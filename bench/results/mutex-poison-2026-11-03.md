# Mutex.with without the copy (decision 0482), 2026-11-03

Xo Lang 2. Base: 93734c344 (0482's parent). New: d87854150 (0482), with
bb59293e3 (the heap list head read with acquire) on top for the --ab rows.

## Mac repro (darwin/arm64, single runs, /usr/bin/time; not an --ab claim)

A `Mutex[List[Int]]` holding 1,000,000 elements; 20,000 `with` calls,
each writing one element (`l[i % size] = i * 3 + 1`); then the checksum
of every element, sum of `x * (k + 1)`, read through `with_read`. The
write changes the list, so a dropped write would change the checksum.

| variant | base wall | new wall | checksum (both) |
|---|---|---|---|
| write, then a read in the closure (mw) | 9.04 s | 0.00 s | 333338666866330000 |
| write as the closure's last statement (mw2) | 8.86 s | 0.00 s | 333338666866330000 |
| store only (mwst) | 9.10 s | 0.00 s | 333338666866330000 |

Before the fix, every `with` whose closure could fault kept a copy of the
value to hand back on an unwind (pack_orig): XO_PROF on the base showed
89% in platform_memmove under obj.rl_copy_n and 11% in munmap. With 0482
a closure that cannot be canceled hands back a zero value on its unwind
exit, so nothing keeps the original alive and the list is updated in
place; a fault poisons the mutex instead (golden mutex_poison).

## cookiejar --ab (linux/arm64, second offload host)

`ab-host.sh 93734c344 bb59293e3 cj482-W --rounds=5 -- bench/perf/cookiejar W`
(the first host was held for Lang 1's survey; an A/B ratio on one host is
valid). Rows below from census/ab-cj482.txt.

Linux aarch64, 18 CPUs; n=5 rounds each; ratio B/A with its interval.

| workload | A wall ms | B wall ms | B/A | interval | verdict |
|---|---|---|---|---|---|
| both | 261.2 | 260.8 | 0.998 | 0.992 to 1.002 | within noise |
| get | 146.9 | 144.2 | 0.982 | 0.976 to 1.005 | within noise |
| set | 89.48 | 89.34 | 0.998 | 0.967 to 1.012 | within noise |

cookiejar's `with` closures store small values, so the copy 0482 removes
was cheap there: no change, as expected. The Go twin row (against Go's
net/http/cookiejar) waits for the first host, for comparability with the
other twin rows.

Gate on the same host (gate-self, bb59293e3): xobuild, self build, std
corpora prelude, http, json on every corpus backend, repotest, stdindex,
stdgen -check, goldens mutex_poison, fault_tasks, conc_mutex_move,
byref_fault_merge, conc_reentrant, conc_unwind, http_h2c,
http_client_pool_body on arm64, LLVM (with the TSan and ASan task runs)
and Go parity, and TestGoldenTasks/conc_unwind under TSan 20 times: all
pass. The first gate run (1a0e42496) failed conc_unwind with a TSan race
on the heap list head, fixed by bb59293e3 (gotchas/heap-list-head-is-newest-record.md).
