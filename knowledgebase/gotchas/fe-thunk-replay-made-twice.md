# A front end entry that made a thunk refused to replay

Status: Observed (2026-10-23, fixed)

## Symptom

internal/arm64 TestModuleCheckAuto failed on main for dce_values: "the
module check cache skipped 1 module(s) a body of which was built from
source"; XO_FE_STATS showed `miss.thunk` for `main` on every warm build.

## Cause

Event 7 of a front end entry records a function value's thunk and
whether the body made it, meaning "this body made it", on every
reference (fecache.xo: `x >= r.nf`). dce_values' main names `inc` in a
list and again in an if's arms, so its entry holds two made=true events
for `inc.value`. The replay check required `made == !there`, and the
second event found the thunk the first had just made (tnew), so the
whole entry was refused and main was built from source; with the module
check cache it then fell back.

## Fix

The replay accepts a repeated made event for a thunk the same entry
made, and a thunk another body made first when it is the same one (name,
target function and, when the callee is declared, signature;
thunk_replay_ok). Reading the recording and the replay side by side is
the check for any new event kind: the replay must accept every sequence
the recording writes.
