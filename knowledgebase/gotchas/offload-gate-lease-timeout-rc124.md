# offload-gate.sh stopped a multi-step gate at 15 minutes with rc 124 and no result

Status: Observed 2026-11-08 (Xo Lang 4: cross plus llvm)

## Symptom

`offload-gate.sh WT cross llvm` exits 124 after 15 minutes. A wrapper that shows only the last lines (`| tail`) prints nothing, since the run was stopped before gate.sh printed its summary.

## Cause

The lease length was a fixed OFF_MAX_MIN=15, written for one short step. The llvm step alone takes 20 to 40 minutes.

## Fix

OFF_MAX_MIN defaults from the steps (15, plus 40 for llvm, 35 arm64, 25 cmd, 30 corpora-all, 15 corpora, gen, fp, 3 for others); OFF_MAX_MIN still overrides it. A lease over 30 minutes prints a note (the lead's rule is 30 at most per lease: split the steps over two runs). On rc 124 the script says so and names the log, which keeps the step lines that finished. Do not pipe it through `tail`: the lines stream as the steps finish.

Observed again 2026-11-15 (Lang 3, fixed by Lang 2): a `repo stdcov` gate got 21 minutes on busy host 2 and stopped at rc 124 with no step line. The host had the tree's xo, so the lease was right to add no build time. The cause was that stdcov (TestStdAPICovered over the resolved sweep) takes 521 s even on quiet host 3 but fell under "3 for others". stdcov now adds 15 and irmanifest 12 (460 to 643 s measured). When you add a gate.sh step, give it a budget line from the lead's notes/gate-times.
