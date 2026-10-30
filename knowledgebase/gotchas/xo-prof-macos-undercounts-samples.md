# XO_PROF on macOS delivers far fewer samples than its rate asks for

Observed 2026-10-30 (Xo Lang 2). Status: Observed.

**Symptom.** A metricsum run of about 10 s of CPU (20M observations,
`XO_PROF=cpu XO_PROF_HZ=1000`) wrote a profile with 2,616 samples, about
25% of the 10,000 the rate implies; a run of about 1 s at 10 kHz wrote
175, under 2% of the 10,000 asked for. The shares were consistent across
runs (qinsert 51% cumulative, rl_copy 25% under it), so the ranking held.

**Cause.** Not confirmed. The likely one: macOS delivers SIGPROF to the
process, not to a thread, and coalesces a signal that arrives while one
is pending, so ticks are dropped, more of them at a high rate where the
handler's own unwind is long (profiler-high-rate-distortion.md). The
sampler fix is queued with the lead.

**Fix (until then).** Read an XO_PROF profile from macOS for shares
(which function, what fraction), never for absolute time or sample
counts: time a phase with `--phase-times` or `xo bench --ab`, and count
work with instruction counts (callgrind) or a counter in the code. Keep
the rate at 1,000 Hz or below and merge several runs when a function's
share is small.
