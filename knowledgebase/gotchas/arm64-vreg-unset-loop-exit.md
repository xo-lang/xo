# arm64 backend: "reads virtual register N before it is written" on one target only

Status: Observed and fixed (2026-10-30)

**Symptom.** Cross-building xo for linux/arm64 (`xo build --backend=arm64
--targets=linux/arm64`) stopped with `std/native/llrt/proc.xo:812:3: in
proc.wait_exit: arm64 backend: internal: proc.wait_exit reads virtual
register 25 before it is written`. The darwin/arm64 build of the same
tree succeeded, so the Mac gates passed.

**Cause.** compiler/arm64 isel.xo lowers a branch it decides (`while
true`, a constant) as a jump through the kept edge's moves. When a value
on the kept path has its landing check in a block only the other edge
reaches (pads_reached false: chain_pads shares one check), that block
stays a second jump target for reachability, and that target was the
block itself, without the edge's moves. When the other edge carries the
loop's values (the exit of `while true`), the register check found them
read on a path where nothing wrote them. A callee that faults on every
target but one (llrt darwin.wait4_ru seen from linux) puts such a check
in the loop on that target only, so darwin built and linux did not.
Reduced to golden loop_exit_decided.

**Fix.** The second target goes through the edge's moves (g.edge), as
the rt.landing branch's two targets already did. internal/arm64
TestLinuxEmitDecidedExit emits the golden for linux/arm64. A std/native
or llrt change also builds xo for linux/arm64 before it pushes when it
has target-specific calls: the darwin gates do not see the other
target's folds.
