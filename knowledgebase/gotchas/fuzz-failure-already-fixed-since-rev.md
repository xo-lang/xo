# A long fuzz failure was already fixed on main

Status: Observed 2026-11-18 (task #152).

**Symptom.** A 200 minute std/sql TestPgFuzz run on linux/arm64 host 2
failed at seed 38 case 2618 (arm64: "types" and nothing after it, where
pgx printed "affected 0"). The same seed passed 2 of 2 on darwin, so it
was reported to the lead as a linux only or timing dependent failure.

**Cause.** The run checked out origin/main at its start (c50a1b0f2e,
21:28Z). The fix, 807f802e1 (a ParameterDescription takes its types from
its length; TestPgSelectNoColumns names this very case), landed on main
29 minutes later, while the run was still going. The darwin repro ran at
a newer main that had the fix. Nothing was platform specific.

**Fix.** Every fuzz, stress or soak result line names the rev it ran at
(stdproven's log prints it as `stdproven NAME: REV`). Before reporting a
failure, check what landed since that rev:
`git log --oneline REV..origin/main -- std/MODULE internal/stdtest` and
`git log origin/main -S'seed N case M'` (fix commits and their regression
tests name the seed and case). A repro on another host runs at the same
rev as the failure (`git checkout REV`), not at current main.
