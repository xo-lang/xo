# arm64: jump threading into a loop's body makes a second entry, and hoist_consts then moves code into another loop

Status: Observed (2026-10-18, xo-4, branch xo4-audit2 before its push);
fixed.

## Symptom

`xo build` of the TLS client stopped with `tlsname.ip_string reads
virtual register 38 before it is written` after thread_const (opt.xo)
was made to iterate to a fixpoint. The compiler driver (`xo build
compiler/arm64`) did not reproduce it; real xo builds bisected it.

## Cause

Threading a jump from outside a loop past the loop's header straight
into its body gave the loop two entries (an irreducible loop).
hoist_consts computes natural loops from back edges and takes the
preheader as the header's unique outside predecessor; with the extra
entry its loop body reached blocks of another loop, and a constant was
hoisted to where one path never defined it.

## Fix

thread_const recomputes the loops (dfs_loops) at the start of each
round, and lets a threaded path enter a loop header from outside only
when the header has no other outside predecessor and every block on the
path before it has a single predecessor; a round ends after such an
entry. A pass that changes edges inside the MIR must keep every loop
reducible, or rerun its loop analysis.
