# A `match` on `target` costs compares on every call on arm64

Status: Observed (2026-10-13, decision 0213's Clock.now on macOS)

Symptom: Clock.now from Xo on macOS (std/native/llrt/sysio.xo choosing
its OS layer with a `match target.os` and `match target.arch`, then the
virtual clock test and clock_gettime_nsec_np) took 1.042 times the C
path's instructions on the arm64 backend, 0.934 on LLVM
(bench/results/platform-calls-2026-10-13.md). The disassembly of the
layer choice is `mov x9, #1; cmp x9, #2; b.eq ...; cmp x9, #1; ...`:
a compare chain over a constant on every call. Writing `==` on the
enum instead called the enum equality helper (1.076).

Cause: the SSA passes do not fold a `match` on the predeclared `target`
(every arm is lowered, gotchas/wasm-target-os-is-the-host.md), and the
arm64 emitter does not fold a branch or switch on a constant; LLVM's
optimizer does.

Fix: keep the choice of an OS layer out of per call hot paths on arm64:
choose it once and keep it in a word, as sysio.xo's clocks do since
work item B01 (2026-10-14: word 0 of rt.sys_words holds the vDSO
address on Linux or a negative code naming libSystem or kernel32, set
by the first read; bench/results/clocks-2026-10-14.md). Or fold constant switches:
a branch on a constant tag in selfhost/arm64's isel (mirrored in
internal/arm64 while the differential tests compare the two) would turn
the chain into one jump; then remeasure `WORKLOADS=clock
bench/perf/sysio/measure.sh`.

Fixed 2026-10-16 (decision 0246): the SSA builders fold a `match` or
`if` on `target`, so the arm64 emitter sees only the taken arm and no
compare chain.
