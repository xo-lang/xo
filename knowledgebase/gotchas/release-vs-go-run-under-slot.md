# release-vs-go.sh under a heavy.sh slot: every run row "(failed)"

Status: Observed (2026-11-21, 0.55.0's Mac report, task #645; fixed in task #686)

Symptom: bench/results/release-vs-go-v0.55.0.md's run section has 20 rows,
each with one line, "xo bench: run --ab as its own HEAVY_CLASS=measure job
(a gate or slot would deadlock against a measure window); this run is
heavy.sh's heavy.2", then "(failed)". The script still exited 0 and its
compile section was fine, so the report was committed as it was.

Cause: the run section had been started as a heavy.sh slot job (heavy.2),
not as HEAVY_CLASS=measure. `xo bench --ab` refuses there
(bench-ab-inside-gate-deadlock.md), and the script wrote each refusal as a
failed row and went on.

Fix: since task #686 the script exits 2 before any work when SECTIONS
holds run and XO_HEAVY_SLOT is set to anything but measure
(TestReleaseVsGoRunRefusesSlot). Run the run section as its own
`HEAVY_CLASS=measure HEAVY_DARWIN=1 heavy.sh` job (toolchain.md, release
step 7), the other sections wherever they fit.
