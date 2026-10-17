# arm64: if-conversion that removes mispredicts can still cost cycles

Status: Observed (2026-10-17, compiler/arm64 isel.xo select_branch and
logic_value; bench/results/arm64-float-text-2026-10-17.md)

Symptom: after csel if-conversion, simulated branch mispredicts fell
(text zone_offset 1.03M to 0.66M) but cycles rose: zone_offset 1.17x,
json enc_strings 1.09x, zone_civil 1.04x; the sweep (instructions)
showed 45 of 114 workloads slower.

Cause: three shapes where a select is worse than a predicted branch:

- a condition on a loaded value in a loop (a binary search's `if sec <
  w[m] { hi = m } else { lo = m + 1 }`): the select makes the next
  iteration's load address wait for this load, where a predicted branch
  lets the core run ahead;
- the second half of `a && b` or `a || b` speculated (computed every
  time) only to give a Bool that the join then branches on anyway;
- `y % 4 == 0 && (y % 100 != 0 || y % 400 == 0)` as a Bool value: every
  test runs, each a multiply high or a division.

Fix: select_branch skips a condition that comes from a call or a memory
read within three operations (from_memory) and a join that only
branches on the selected value; spec_ok does not speculate compares;
logic_value skips a test that divides (divides). Measure every
if-conversion change in cycles, not instructions only, and on the
sweep's slowest rows (gotchas/arm64-fewer-instructions-slower.md).
