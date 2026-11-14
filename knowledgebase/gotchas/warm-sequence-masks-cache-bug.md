# A long warm build sequence can hide a cache bug a pair shows

Status: Observed (2026-11-14)

## Symptom

TestWarmBuildSequence's first version built 16 std goldens one after another
through one default cache, in name order, reversed and shuffled. With the xo
of b8c13a7c8's parent (the 0.51.0 respin's front end cache bug) all three
orders passed, though `log_state` then `log_error` alone fails with XO1201.

## Cause

The bug needs a particular entry to be read before anything rewrites it. In
name order log_error is built before log_state; reversed, log_sinks, log_lazy
and log_handlers are built between log_state and log_error, and one of them
records the entries log_error needs, so the bad replay never happens.

## Fix

Check pairs too: every ordered pair A then B of programs sharing a std module,
each pair on its own fresh cache (decision 0631). The pairs find the respin's
bug; the sequences find accumulation effects. A cache test that only fills one
long sequence is not evidence that two builds in a row work.
