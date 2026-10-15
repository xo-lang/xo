# An arm64 template ending in cmp and cset loses its cset to a branch

Status: Observed (2026-10-15, decision 0232's rt.sym_present).

## Symptom

The arm64 emitter faulted while compiling every program
(`index 0 out of range for List of length 0 at arm64/emit.xo`): an
instruction's text used `$d0` but the instruction had no definition.

## Cause

rt.sym_present was one def1 template, `adrp ...; ldr $d0, ...; cmp $d0,
#0; cset $d0, ne`. When the result only feeds a branch, an optimization
pass drops the template's trailing cset and its definition and branches
on the flags, leaving the earlier lines that still name `$d0`.

## Fix

Keep a compare and cset a template of their own: the value first (a
def1), then `cmp $u0, #0\ncset $d0, ne` over it, the shape the pass
expects. Write any new flag producing intrinsic that way.
