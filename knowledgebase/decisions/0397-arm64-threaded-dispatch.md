# 0397: Threaded dispatch: a jump table loop's dispatch copied into each arm

Status: Proposed (2026-10-28, xo-lang-6; stub, filled in with the change)

## Context

interp's loop dispatches a 16 op program through one jump table `br`
(decision 0051); the same code ran 204.5M to 281.4M cycles over 16
placements (bench/results/interp-placement-2026-10-28.md).

## Decision

(to be filled in: compiler/arm64 threadsw.xo thread_switch)
