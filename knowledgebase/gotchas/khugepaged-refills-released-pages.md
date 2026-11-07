# khugepaged refills pages the heap gave back, and RSS rises at idle

Status: Observed (2026-11-06 and 2026-11-07, Xo Lang 2, linux/arm64 offload host 1)

Symptom: a native Xo server's RSS rises by 8 to 16 MB while it is idle,
with no requests and no allocation, and `AnonHugePages` in
`/proc/PID/smaps_rollup` rises with it (0 to 14 or 20 MiB). It happens in
some runs, not all (2 of 8 sat128 runs of stability/notes, one in each
build), so a single run reads as a regression in whatever change is under
test, including one that gives pages back (decision 0533).

Cause: Linux transparent huge pages with `enabled=always` and khugepaged's
`max_ptes_none` at 511, the kernel default
(`/sys/kernel/mm/transparent_hugepage/khugepaged/max_ptes_none`).
khugepaged scans eligible anonymous mappings (every 10 s here) and
collapses any 2 MiB aligned range that has at least one resident page into
a huge page, filling the up to 511 missing pages with zeroed memory. A heap
range given back with MADV_DONTNEED stays eligible, so the collapse undoes
the release; a sparsely touched range that was never given back is filled
the same way.

Fix: measure RSS rows with `XO_THP=off` beside the default (`dense`,
decision 0456, whose chunks take huge pages once a heap record fills
2 MiB; `off` is decision 0454's:
MADV_NOHUGEPAGE on every heap mapping). With it no run re-inflated (0 of
8) and AnonHugePages stayed 0. Read `anon_huge_kib` in the harness's
mem.txt and samples.csv before calling an idle rise a leak. Decision 0533's
amendment skips dense 2 MiB ranges (age_sparse). Advising MADV_NOHUGEPAGE
on each range it gave back did not help (3 of 4 runs still rose, against
2 of 4 for the base): most collapses are in ranges never given back, so the
fix is XO_THP=off or host tuning, not the scavenger. Check the host's settings
first:
`cat /sys/kernel/mm/transparent_hugepage/{enabled,defrag,khugepaged/max_ptes_none}`.
Go has the same exposure and the same knob (GODEBUG=disablethp=1).
