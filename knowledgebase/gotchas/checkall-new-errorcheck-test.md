# A new errorcheck test failed the gate's checkall step

Status: Observed (Xo Lang 3, 2026-11-16)

## Symptom

The cbortag gate (selfbuild seedbuild stdfast checkall) failed with
`checkall: FAIL (6 errors the merge base's xo does not report)`, every
one in the new `test/errorcheck/cbor_tag/main.xo`: the errors the test
expects.

## Cause

scripts/checkall.sh lists every error `xo check` reports over the
repository's programs, and gate.sh prove_checkall fails on the ones only
the change's xo reports. A new errorcheck program's errors are always
new (the merge base has no such file), so any push adding an errorcheck
test failed the step.

## Fix

checkall.sh leaves out an error whose line carries `// ERROR` naming its
code (the errorcheck annotation, which the errorcheck test itself
checks). TestCheckallExpectedErrors (scripts/checkall_test.sh) covers
it.
