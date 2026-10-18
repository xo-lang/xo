# A gate goes on with a stale bin/xo after its build fails

Status: Observed (2026-10-18, Xo Lang 3)

## Symptom

A gate script reports test failures (or passes) that have nothing to do
with the change: on 2026-10-18 a gate for `xo run --watch`, XO0531, and
`xo dev compare` failed TestDevCompare, TestVetCLI, and TestVersion,
all on an xo built before those commits (it had no `xo dev compare`).
The build line near the top of the log said `build=1`. (The same gate's
25 minute `xo vet compiler` was a real bug, in XO0530's heap field count,
which the older build had too; it was found by the vet step, not caused
by the stale binary.)

## Cause

`go run ./scripts/xobuild` failed (a check error in the change: a
variable named `uses`, a keyword), so it wrote no new bin/xo. The script
logged the status and went on, and every later step ran with
`XO_CLI=bin/xo`, the binary of the last build that succeeded. Tests then
measure the old compiler: a pass proves nothing about the change, and a
failure (a test of a command the old xo lacks) points the wrong way.
With `XO_CLI` unset, xotool would rebuild and fail loudly; a gate sets it
to bin/xo to skip that rebuild, which is what hides the failure.

## Fix

A gate stops when its build fails. scripts/gate.sh always did
(`XOBUILD-FAIL`); scripts/gate-http.sh now writes `gate=stopped` and
exits after `build=N` with N not 0; a scratch gate script uses
`go run ./scripts/xobuild ...; b=$?; [ $b = 0 ] || exit 1` before its
first test. When reading a gate log, check `build=0` before any other
line.
