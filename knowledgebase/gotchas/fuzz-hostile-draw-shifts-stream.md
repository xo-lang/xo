# A hostile branch added to a fuzz generator shifts every later case

Status: Observed (2026-11-18, task #352, mime and mail harnesses)

## Symptom

After adding a hostile dictionary branch (`hostile.Pick(r, ...)` or a new
`r.Intn`) to an internal/stdtest fuzz generator, the corpus test fails a
coverage floor that has nothing to do with the new branch: mime's
`format` fell from 100 distinct answers to 91, mail's msg cases from 34
failures to under the 10% floor (380 of 414 ok).

## Cause

The generator draws from one `math/rand` stream. Every draw the new
branch makes (the branch's own `r.Intn`, then `Pick`'s) moves every case
after it, so seed 1's corpus is a different sample and a floor tuned to
the old sample can fall below its line.

## Fix

Draw as little as possible from r:

- Put the new branch inside an existing `switch r.Intn(N)` (a case that
  was a no-op or a default), so choosing it costs no new draw (mime).
- Choose the hostile value from data already at hand instead of r, such
  as `ds[(len(s)+i)%len(ds)]` (mail's `mlMutate` case 2), so the branch
  draws nothing.

Then rewrite the corpus (`XO_<NAME>FUZZ_WRITE=1`) and check the floors
still hold.
