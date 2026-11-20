# A private XO_BACKEND_CACHE_DIR in a measurement builds the whole program, not what users run

Status: Observed (2026-11-20, tasks #480 and #508)

## Symptom

The first ADR 0727 baseline (bench/results/native-vs-llvm-2026-11-20.md
at 8f5a6531d) ranked fault defer first, at 1.725 times LLVM's
instructions on arm64. A profile of the same bench at the same commit
then counted 940M instructions, not 1,594M: level with LLVM. Other rows
moved the other way: portable checked was 1.038 in the file and 3.036
in the default build, atomic add 0.719 and 1.687.

## Cause

The baseline script gave every native build an empty
`XO_BACKEND_CACHE_DIR` of its own, to time a cold backend. With no std
variant in that directory the build says "std variant: miss, building
in the background, the whole program is built" and compiles std and
the program as one module. The default build reuses the separately
compiled std ("cache std ... reused") and its code is not the same:
std functions the program calls are inlined into it in a whole program
build, and the inliner's choices inside std differ too (the unwinder's
uleb reader was a function of its own in the whole program build, 24%
of fault defer). `XO_BACKEND_CACHE=off` and `XO_SEPCOMP=0` build the
whole program as well.

## Fix

A measurement of what users run builds with the host's default caches
and checks its build log for "cache std ... reused" (`XO_CACHE_STATS=1`
prints the cache lines); a whole program build is a labeled diagnostic
(a first build on a clean cache), never the headline. To time a cold
backend, time it in a build of its own and measure another binary. The
corrected file gives both columns.
