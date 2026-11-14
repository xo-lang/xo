# A merged std file lost its generator's END marker

Status: Observed (2026-11-14, Xo Lang 3)

## Symptom

`go run ./scripts/genidna NETDIR v0.43.0 std/idna.xo` failed with
`panic: markers missing` inside the temporary module's TestDump. A
relative FILE fails the same way, since the dump runs in a temporary
directory and cannot find the file.

## Cause

Moving std/native/idna.xo into std/idna.xo (decisions 0588, 0597, 695fac328)
kept `// BEGIN GENERATED` but dropped `// END GENERATED`, so the generator
had nowhere to stop. Nothing runs the generator in a gate, so the loss went
unnoticed until the tables needed regenerating.

## Fix

Restore the marker after the last generated table (std/idna.xo, before
`const MAPPED`) and pass FILE as an absolute path (`$PWD/std/idna.xo`).
After a one-file merge of a module with generated tables (idna, norm and
the rest), check both markers survive:
`grep -c "GENERATED" FILE` should print 2.
