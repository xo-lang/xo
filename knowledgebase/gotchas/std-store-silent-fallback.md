# A std edit that only the store's full check rejects turned every separately compiled build whole-program in silence

Status: Observed (2026-10-29; 48da2bb2b, std/native/http.xo:2251 use of a moved value).

## Symptom

For hours every default build on main said "std variant: miss" (only
with --phase-times) and built the whole program: the std records' tests
and every separately compiled check failed with "xo dev stdcache: exit
status 1", while whole-program builds and the goldens passed.

## Cause

A whole-program build checks std's bodies lazily (the ones it reaches);
`xo dev stdobj` (the store) checks every body, and rejected a moved
BytesBuilder in the new ClientStream code. The variant's background
build failed, its reason went only to the detached child's log, and
each build that found no store started another background build and
went whole-program, "never slower than without separate compilation".

## Fix

The failed build leaves `<key>.store.failed` with the first error line
beside the store's name; a build that finds it prints `xo: warning:
separate compilation is off: std's store build failed (FILE:LINE: ...)`
and builds whole. Since 2026-11-04 (b26 of the owner's v0.38.1
evaluation) the marker also names the std tree that failed (`std PATH`),
and it holds only for that tree and for an hour: the key follows std's
relative paths, sizes and times, not its place, so a deleted release
smoke test's copy under $TMPDIR (no zoneinfo.zip there) shared the
intact install's key and turned its separate compilation off for good.
A marker of another tree, of an older xo (no `std` line) or past its
hour is removed and the build tries again; the warning names the
marker's path. internal/arm64 TestStdFailedMarkerPerTree. internal/arm64 TestStdStoreBuilds builds the store from scratch
and is not skipped under -short, so the hourly check sees it.

## Rule

A fallback that costs only speed still needs one line: a silent one
hides a broken path until a test that needs the path fails somewhere
else. Any std change gates on the store build (xo dev stdcache), not
only on the goldens.
