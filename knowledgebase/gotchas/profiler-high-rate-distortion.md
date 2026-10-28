# XO_PROF at 10 kHz slows the process 5 to 10x and misattributes task frames

Observed 2026-10-28 (xo-lang-6). Status: Observed.

**Symptom.** A fib edit build (109 ms) profiled with `XO_PROF=cpu
XO_PROF_HZ=10000` took 586 to 1,083 ms, and the merged profile charged
11 ms per edit of `parser.iface_text_lex` to `std_tree_sum`, which an
instrumented run showed re-lexed nothing (its size and mtime memo hit
every file).

**Cause.** 10,000 SIGPROF signals a second of CPU is a signal every
100 us: the handler's unwind costs more than the work between samples,
so the process crawls, and a sample taken while a task fiber runs on
another thread can be attributed to the main fiber's current frame
(the walk stops at the fiber boundary), so a task's lexing lands under
whatever the main thread was doing (std_tree_sum at that moment).

**Fix.** Profile short processes at the default 100 Hz or at most
1,000 Hz, merge many runs (`go tool pprof -proto run1.pprof run2.pprof
... > merged.pprof`, then `-top merged.pprof`), and cross-check any
surprising attribution with an absolute stamp or a counter in the code
(the stamped timeline: an epoch ms printed at main's entry, each phase
lap and before exit, with a perl parent stamping exec and reap, accounts
for every millisecond without a profiler).
