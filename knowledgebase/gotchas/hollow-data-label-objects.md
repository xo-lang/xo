# hollow drops the words under a data helper's unnamed label

Status: Observed (2026-10-31, arm64 zero images, compiler/arm64 zimage.xo).

## Symptom

A helper that emits data (a read only image of words) assembled with
its words missing in a split build: the global the code names was
followed at once by the next helper, so a `memcpy` from it copied
whatever came after. xo built by that compiler failed with SIGBUS
writing into `__DATA_CONST,__const`; a small program faulted in its
first method.

## Cause

hollow.xo (the dead text and data pass before assembly) takes each
label of a data section as an object of its own and keeps only the
objects something names. A code helper's labels are one object because
code falls through, but a data helper written as rc.xo's helpers are,
`${g.alias(key)}${sym}:` then the body, put the words under `sym`, a
local label nothing names in a split build (the parts name the global
that alias writes, helper_ext), so hollow dropped them.

## Fix

Write the label the module's code names last, directly above the
words: in a split build `${sym}:` then alias (whose global comes
last), in a whole build alias then `${sym}:`. Check a data helper's
words in the kept assembly (`--asm=FILE`) of a split build, not only
of a small program, which is not split.
