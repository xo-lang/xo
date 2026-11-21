# A cached-xo probe must test what xotool.Ensure tests

Status: Anticipated 2026-11-21 (task #585; the cut that prompted it had another cause).

## Symptom

offload-gate.sh probes `offload.sh --has-xo KEY`, finds the host "warm",
sizes the lease without the cold xo build (OFFGATE_XOBUILD_MIN), and the
gate then builds xo anyway and runs out of lease.

## Cause

`--has-xo` tested only `test -x .../checkout/KEY/xo`. xotool.Ensure
reuses an entry only when its `stamp` file exists and its dirty field is
`-` (a clean tree's build, gotchas/cached-xo-stamped-dirty.md); a
stampless or dirty entry is rebuilt.

## Fix

`--has-xo` runs the same test as Ensure: executable `xo` and a stamp
whose second field is `-` (offload.sh, `hx_cmd`). `OFF_HAS_XO_ROOT=DIR`
runs it locally on a fake cache; offload_gate_test.sh covers clean,
stampless, dirty and xo-less entries. A change to Ensure's reuse rule
changes `hx_cmd` too.

The host 3 cut on 2026-11-21 (xw1-582, 755 s against a 12 minute lease)
was not this: the entry was clean and built before the gate. It was a
warm history lease (p90 times 1.3) with no margin at Mac load 24; owner
question #240 covers that.
