# A build's instruction count varies run to run: the speculative front end workers

Status: Observed (2026-11-20, task #497)

## Symptom

cgcheck raised build_http +3.55% and build_json +2.72% at 8dd8e32c9
against 4bcf45d5c (xw alert 92233). Bisecting the 33 commits found no
step: the same xo building bench/perf/json under callgrind gave 30.03G
to 30.25G instructions in six runs, six revisions of the range gave
28.24G to 29.96G in no order of the commits, and the scheduled rows had
moved between 27.68G and 29.68G (build_json) and 86.4G and 92.3G
(build_http) over their last ten runs. The base was a low sample.

## Cause

The front end lowers queued bodies ahead of their turn on speculative
workers (decision 0637 stage 3; `fe spec.workers 6` in XO_FE_STATS).
Their number comes from the machine's cores, not from XO_THREADS, so
they run in a one thread build too. How many bodies a worker lowers
before the main task reaches them, and how many of those are used,
depends on the scheduling, so the total work of a build is not the same
twice. callgrind counts every thread.

## Fix

scripts/lead/cgcheck-inside.sh runs with `XO_FE_SPEC=0`. With it the
count repeats within 0.25% (27.91G and 27.98G at 4bcf45d5c, 27.91G and
27.94G at 8dd8e32c9: no regression). Any instruction count of `xo
build` itself (a bisect, a compile time claim) sets `XO_FE_SPEC=0`, with
cgcheck's other settings (`XO_THREADS=1 XO_PREEMPT=0 XO_UNGUARDED=1
XO_SEPCOMP=0`); a wall time claim keeps the default.
