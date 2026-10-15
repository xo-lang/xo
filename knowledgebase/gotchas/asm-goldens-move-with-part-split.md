# The arm64 assembly goldens change when the runtime grows past a part boundary

Status: Observed 2026-10-14 (B19 part 2, dcd9c609).

**Symptom.** `TestGoldenAsm` (internal/arm64, fib and sieve, darwin and
linux) fails after a change that touched only the runtime (std/native),
although `ownFuncs` leaves the runtime's functions out of the goldens. The
diff renames every label of `main` (`Lf1_b0` becomes `LpGL1jtk_f1_b0`,
`Lxo_str0` becomes `Lxo_pGL1jtk_str0`, the LSDA table gains the prefix),
adds `.globl`/`.private_extern` lines for `_xo_f_main`, and turns
`Lxo_static` into `_xo_static`.

**Cause.** Decision 0220 splits a program's functions into parts
(selfhost/arm64/parallel.xo: PART_MIN, PART_CUT, PART_MAX by function count
and name hash). More runtime functions move the part boundaries, so the
program's own functions land in a different part, whose labels carry the
part's namespace and whose cross-part symbols are hidden globals
(split-assembly-needs-hidden-globals.md). The instructions do not change.

**Fix.** Check that only symbol names moved (strip labels and symbols from
both sides and diff), then regenerate with
`XO_CLI=<that commit's xo> go test -short -run TestGoldenAsm ./internal/arm64 -update`
in the same commit as the runtime change. A diff in instructions is a real
codegen change and needs review.
