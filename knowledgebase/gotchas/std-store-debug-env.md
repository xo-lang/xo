# XO_PASSES and XO_DUMP_FN do not work on a std store build

Status: Observed (2026-10-30)

## Symptom

Chasing a failure that only `xo dev stdcache` (the std store build behind
internal/arm64 TestStdStoreBuilds) shows, setting `XO_PASSES=-NAME` fails every
run with `xo dev stdobj exit 1: xo: XO_STD_FACTS_DUMP=.../std.stdf is not written
by this build: its reference counting does not take the backend cache's path
(XO_BACKEND_CACHE=off, XO_PASSES, --release, or a program of under 256
functions)`. `XO_DUMP_FN` gives the same refusal (it turns the backend cache off,
decision 0437) and writes no dump.

## Cause

The store writes the std facts file from the backend cache's reference counting
path; XO_PASSES and XO_DUMP_FN both take the build off that path, so the store
build refuses instead of writing facts that would not match.

## Fix / how to work around

- Check the exit status and the message: a refused run has no verifier message,
  so a pass sweep can read as "pass N fixes it" when nothing was built.
- What still works on the store build: XO_INLINE_DENY=a,b (keep those callees
  out). XO_INLINE_REPORT=NAME runs, but its lines go to the `xo dev stdobj`
  child's standard error, which stdcache does not show.
- To see a function's SSA there, build a scratch xo with a debug print at the
  failure (compiler/xo main.xo inline_verified) instead of XO_DUMP_FN.
- A whole program build of a program that calls the function is no substitute:
  it inlines less of std than the store does (the HTTP goldens passed while the store failed, 2026-10-30).
