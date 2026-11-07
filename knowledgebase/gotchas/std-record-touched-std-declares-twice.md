# A touched std file declared every std name twice

Status: Observed (2026-11-07, the lead's tier 58, TestUnwind/fiber/sephalf;
reproduced by hand the same day).

## Symptom

A separately compiled build (XO_SEPCOMP=1, so the std record of decision
0388 stage 2 is on) fails with XO0603 on every top level name of std,
each one's "previous declaration" being itself:

```
std/native/llrt/alloc.xo:45:7: error XO0603: `ALLOCS` is already declared in this module
  note: std/native/llrt/alloc.xo:45:7: previous declaration
```

about 7000 errors, on the second build into a back-end cache after a std
file's modification time changed with its bytes unchanged (a fresh
checkout, `touch`, an editor that saves without a change). The tier hit
it on a std checked out minutes before; the same test passed on every
other run.

## Cause

The stdx entry's header records std's file stamps. When they fail
(`cache stdx header failed`), std loads from source, so no module is
`from_record`; the fingerprint of std's bytes then still matches the
record, and `install_std` installs the record's members into those
source-loaded modules. The branch after it declared every module not
`from_record` again, so every name met itself.

## Fix

(The same touch also changed std's separately compiled variant key,
which held each file's time, so a build right after `xo dev stdcache`
could miss the variant it had just built; the key now holds each file's
content hash, TestStdVariantKeyTouched.)

`install_std` marks the modules whose members it installed
(`record_members`), and the declare, resolve_types and resolve_sigs
passes after it skip them as they skip `from_record` ones.
TestStdRecordTouchedStd (internal/arm64) builds into a cache, touches a
copy of std's task.xo and builds again. `XO_STD_EXPORT=0` avoided it
before the fix. To reproduce such a case by hand, build into a private
XO_BACKEND_CACHE_DIR on a copy of std and touch one file between builds.
