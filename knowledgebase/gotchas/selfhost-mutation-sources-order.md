# The selfhost mutation tests pick their programs by position

Status: Observed (2026-10-13, decision 0215).

## Symptom

After the selfhost tests moved off the Go checker, TestSelfhostCheckerMutations
failed under -short: ten mutations (m0027, m0042, ...) "differ" from the
recording, each a different program from the one recorded under its
name.

## Cause

The mutation tests draw their sources with a seeded random index into
the list mutationSources returns, so the mutated programs depend on
that list's order and membership. The list was the accepted single
directory modules in moduleDirs order; the port chose them from the Xo
checker's batched runs (runXoBatches), which come back cost sorted, so
the order changed and m0027 became another program. The replay then
compared it with the recording of the old m0027 through its slot (the
name).

## Fix

mutationSources takes the candidates in moduleDirs order and each
verdict from the recording (goCheck, as TestSelfhostChecker reads it),
so the sources are the recorded ones. An input in the test's temporary
directory also keeps its content in its slot (goldSlot), so a mutation
of other content never falls back to a recording of the same name: it
is a missing unit, to record.
