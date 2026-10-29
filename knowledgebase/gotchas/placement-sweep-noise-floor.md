# A placement sweep's spread needs the one-binary control: a 50 ms run varies 14 percent between runs

Status: Observed (2026-10-29, xo-lang-6, decision 0403's measurements).

## Symptom

interp over 16 placements (XO_ARM64_PAD) showed a 7 to 14 percent
spread of per-placement median cycles in every sweep that day, with
and without alignment, with the hot function at a page boundary, with a
profile and without; every "fix" left it in place and the per-pad medians
had no pattern.

## Cause

The same binary measured as if it were 16 placements (16 slots x 7
rounds, a quiet window) gave the same spread: 1.139. The variation was
the run-to-run noise of a 50 ms program, not placement. The 38 percent
spread of 2026-10-28 was real (even and odd pads were distinct, and
threaded dispatch removed it); the residue is noise that a 7 round
median does not average out at that run length.

## Fix

Beside every placement sweep, run the one-binary control with the same
statistic; a spread is placement only above that floor (strings: 1.060
against a floor of 1.023, a real 4 percent). Or make the run long
enough that the floor falls under a percent (collatz's 600 ms: 1.008).

## Rule

A spread without its noise floor is not a measurement. Record the floor
in the results file next to the sweep. toolchain.md's placement section
said "16 copies of one binary as the noise floor" since 0075; it was
not applied, so the rule is in agent-tools.md "Measuring" now too.
