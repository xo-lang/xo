# The register order of a callee saved `stp` pair changes speed

Status: Observed (2026-10-08, internal/arm64, bench fib on Apple M4 Pro)

Symptom: the native backend's recursive fib took 305 ms against the LLVM
backend's 222 ms although both functions had the same instructions: save
x19 and x20 and the frame record, two calls, a checked add, restore.
Moving the frame record, adding `.p2align 4`, dropping `mov x29, sp`, or
reordering the epilogue changed nothing (all within 2 percent).

Cause: the order of the two registers inside the pair. `stp x19, x20,
[sp, #16]` / `ldp x19, x20, [sp, #16]` ran in 305 ms; `stp x20, x19` /
`ldp x20, x19` (x19 in the higher slot, LLVM's order) ran in 217 ms, the
same in every frame layout tried. x19 holds `n`, which the caller reads
right after the callee restores it; the restore apparently reaches it
faster from the second slot of the pair. The mechanism is a guess
(memory renaming or load pair handling of the core); the measurement is
not: three runs each, best of 7, interleaved.

Fix: `internal/arm64/emit.go` (`saves`) stores pairs with the higher
register first, as LLVM does, and the allocator hands out x19 first, so
the most used callee saved register sits in the second slot. To check a
similar effect, edit the generated `.s` by hand (the build directory
keeps `prog-darwin.s`) and link it with `cc -O2 prog.s xo_rt.c`; the
script pattern is in `bench/results/arm64-native-2026-10-08.md`.
