# callgrind asserts on xo's preemption signal

Status: Observed (2026-11-18, scripts/lead/cgcheck.sh, task #364, xw-1)

Symptom: `valgrind --tool=callgrind xo build ...` (and `xo check`) on
linux/arm64 (Valgrind 3.24.0, Debian container on the Mac) died with
`Callgrind: threads.c:247 (vgCallgrind_post_signal): Assertion 'sigNum
== vgCallgrind_current_state.sig' failed.` and printed no "Collected"
count. Small benchmark programs under the same callgrind ran fine;
`--fair-sched=yes` did not help, and `--tool=none` ran xo to the end.

Cause: the runtime's preemption (decision 0286) signals a worker that
runs too long without yielding; xo itself runs long enough for it to
fire, small programs do not. Callgrind's signal bookkeeping loses track
of the signal delivered to that thread and asserts.

Fix: run xo (or any long Xo program) under callgrind with
`XO_PREEMPT=0`: no worker is asked to yield, the polls stay and never
fire, and the count is steady (xo build of bench/perf/json: 28.9G
instructions). scripts/lead/cgcheck-inside.sh sets it for every row.
