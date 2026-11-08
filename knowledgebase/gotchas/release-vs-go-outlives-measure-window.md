# release-vs-go's Mac sections outlive a measure window

Status: Observed (2026-11-08, Xo Lang 2).

## Symptom

The 0.44.0 release-vs-go run (`HEAVY_CLASS=measure HEAVY_DARWIN=1
heavy.sh ... scripts/release-vs-go.sh`, sections compile, run, test and
amd64) got its measure window at 16:16 and ran for 90 minutes without
leaving the compile section. Its log repeated `release-vs-go: waiting
for load under 6 (load 15, 27 min of 30)`, and the bulk queue behind it
(the tier's last step and another agent's sweep) waited over an hour.
The 0.41.0 run queued behind it never started.

## Cause

heavyd's measure window holds new gates and slot runs for about 15
minutes (16:16:43 to 16:31:43 that day). The four Mac sections need an
hour or more. When the window closed, gates started again, and
release-vs-go.sh's own guard (wait for a 1-minute load under 6, up to 30
minutes per step, then measure anyway) spent each step waiting at load
12 to 27. Every number it would have written was taken under load, and
the run still held the measurement queue, so heavyd kept holding the
bulk runs behind it.

## Fix

Run the Mac sections in a window as long as the run: the lead opens one
with `~/xo-lead/agents/quiet.sh MINUTES` (heavyd reads `~/xo-lead/quiet`
each tick and holds new gates and slot runs), the run waits with
`quiet.sh wait 6`, and then goes through heavy.sh with `HEAVY_QUIET=1
HEAVY_CLASS=bulk HEAVY_DARWIN=1`, which proceeds inside the window. Size
it at about 75 minutes a release (0.44.0 then 0.41.0 got 01:00 to 03:30
on 2026-11-09). A measure class run of the whole script only suits one
section. Stop a run whose log shows the load guard timing out step after
step, since its numbers are unusable anyway.
