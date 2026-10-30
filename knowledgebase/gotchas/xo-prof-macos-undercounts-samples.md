# XO_PROF on macOS delivers far fewer samples than its rate asks for

Observed 2026-10-30 (Xo Lang 2). Status: Observed, fixed 2026-10-30.

**Symptom.** A metricsum run of about 10 s of CPU (20M observations,
`XO_PROF=cpu XO_PROF_HZ=1000`) wrote a profile with 2,616 samples, about
25% of the 10,000 the rate implies; a run of about 1 s at 10 kHz wrote
175, under 2% of the 10,000 asked for. The shares were consistent across
runs (qinsert 51% cumulative, rl_copy 25% under it), so the ranking held.

**Cause.** The kernel, not Xo (confirmed 2026-10-30, Xo Lang 1). A C
program with an empty SIGPROF handler and setitimer(ITIMER_PROF) gets
0.88 of its samples at 100 Hz, 0.16 (one thread) and 0.23 (four) at
1 kHz, and 0.07 at 10 kHz on the same Mac: macOS delivers ITIMER_PROF
to the process and drops ticks. Linux's ITIMER_PROF is exact for one
thread but is one process-wide timer that fires at most once per kernel
tick: four threads at 1 kHz got 0.69 of their samples, and 10 kHz caps
near 1,000 signals a second.

**Fix.** XO_PROF no longer counts signals (std/changes/debug.md,
2026-10-30). On macOS a sampler thread reads each thread's CPU time
(thread_info) every period and takes one sample per whole period a
thread used, its registers read with thread_suspend and
thread_get_state: a 4-task spin at 1 kHz now gets 0.970 of the samples
its CPU time implies (0.987 for one task; 0.14 and 0.29 before). On
Linux each SIGPROF sample is weighted by the thread's
CLOCK_THREAD_CPUTIME_ID time since its last sample: 4 tasks at 1 kHz
0.999 (was 0.683). internal/llvm
TestProfCountsCPU checks the ratio stays within 0.9 to 1.1. A profile
from an older xo still holds only for shares.
