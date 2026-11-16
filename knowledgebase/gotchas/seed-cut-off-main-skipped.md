# A seed cut while main moved was skipped by every host gate

Status: Observed (2026-11-15)

## Symptom

Seed 0.52.0-38164047 was uploaded and unpacked on every offload host,
but gates there still printed `seed 0.51.1 (newest on the host from an
ancestor of HEAD)`, and a seedbuild that needed 0.52.0 failed (XO0702
unknown effect sync, std/http.xo:549).

## Cause

The cut ran without `XO_PUSH_LOCK`. Main moved during it, so seed-cut.sh
copied the version commit onto the newer main (26d1e5bf) and kept the
seed and the tag on the gated commit 38164047, which is off main.
offload-gate-remote.sh takes a seed only when the commit in its
directory name is an ancestor of HEAD, and the host's repository gets
only branch heads from the gate's bundle, never the tag. xotool on the
Macs was unaffected: its seedBase walks an off-main release commit to
its parent on main.

## Fix

The same day, the lead pointed `OFF_SEED` at the new seed, and a
`xo-0.52.0-26d1e5bf-linux-arm64` link went on the arm64 hosts. Since
then seed-cut.sh refuses an X.Y.0 cut without `XO_PUSH_LOCK` and stops
on a refused push instead of copying the commit. scripts/lead/seed-place.sh
(run by seed-upload.sh) sets `OFF_SEED`, and it links an off-main seed
under main's version commit. Tests: seed_cut_lock_test.sh and
seed_place_test.sh (internal/repotest).
