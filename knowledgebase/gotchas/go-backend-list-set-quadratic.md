# List element writes make Go backend benchmarks quadratic

Status: Observed (2026-10-08, `bench/perf/llvm` sieve, sort, nbody).
Fixed 2026-10-08: lists write in place when they own their array
(`list-own-marks.md`, `bench/results/perf-list-2026-10-08.md`); sieve
runs in 0.15 s (1.4x Go), sort 0.27 s (1.0x), nbody 0.26 s (1.6x).

Symptom: a sieve over a 10M element `List[Bool]` built with the Go
backend (`xo build`) ran for over 10 minutes; the same program built
with `--backend=llvm` takes 0.1 s, as does hand written Go. nbody (five
bodies, 5M steps) took 19 s against Go's 0.15 s.

Cause: the P1 runtime's `xs[i] = x` (`rt.List.SetAt`, also `xs.set`)
copies the whole list on every write (a documented gap in
`infrastructure/toolchain.md`, "Known gaps"). A loop of n writes into an
n element list is O(n^2); for small lists (nbody's five structs) every
write is still an allocation and a copy.

Workaround (before the fix): benchmarks with element writes need a
timeout (`bench/perf/llvm/run.sh` stops a run after `TIMEOUT` seconds and
shows `>60000`; still useful when comparing with older binaries). Fix:
unique ownership for the Go backend's lists, a sticky shared flag set by
own marks in the generated code (`list-own-marks.md`); the Map entry log
technique alone could not work for writes (see the results file).

Related: stopping such a run with `kill <pid>` (SIGTERM) does nothing
visible, since the first SIGTERM only requests a stop
(`first-signal-no-longer-cancels.md`); a busy program needs SIGKILL or
three signals.
