# XO_PROF made every profiled xobuild build twice

Status: Observed (2026-11-05, linux/arm64 host 1, xo-lang-4)

## Symptom

A warm edit rebuilt with `XO_PROF=cpu go run ./scripts/xobuild` built
twice: a warm 11 to 17 s build, then "xotool: the newest earlier build
... is an unknown number of commits behind HEAD (more than 50): building
again with the xo it built" and a cold 43 to 49 s build. The largest
profile was the cold build's, which looked like XO_PROF being in a cache
key. It is not.

## Cause

xotool's commitsBehind ran `xo version --json` through runIn, which
returns standard output and standard error together. Under XO_PROF
every xo prints "xo profile: N samples in ..." to standard error at
exit, so the JSON did not parse, the distance was unknown, and
SecondHop rebuilt with the xo just built (a new codegen key, so cold).
needHeadBuild also missed the clean HEAD build the same way.

## Fix

commitsBehind reads standard output only (internal/xotool xotool.go;
TestNeedHeadBuild has an xo that also writes to standard error).

## How to avoid

When a tool parses another tool's output, read standard output only.
XO_PROF, XO_CACHE_STATS, XO_FE_STATS and --phase-times all write to
standard error.
