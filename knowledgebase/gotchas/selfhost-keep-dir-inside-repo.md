# XO_SELFHOST_KEEP inside the repository adds runs to TestSelfhostChecker

Status: Observed (2026-10-10)

Symptom: TestSelfhostChecker reported 749 runs instead of 599, and the
runs rejected for other reasons included `scratch/keep/check-mutations/
m0077` and others: modules nobody wrote.

Cause: with `XO_SELFHOST_KEEP=<dir>`, TestSelfhostCheckerMutations (also
selected by `-run TestSelfhostChecker` without `$`) writes its mutated
modules to `<dir>/check-mutations/mNNNN`. A dir inside the repository
that is not a dot directory is walked by `moduleDirs` on the next run, so
those modules become runs of TestSelfhostChecker (and of every test that
walks the repository's .xo files).

Fix: point XO_SELFHOST_KEEP at a dot directory (`scratch/.keep`) or
outside the repository, and select the checker test alone with
`-run 'TestSelfhostChecker$'`. Delete a stray keep directory before
reading the run counts.
