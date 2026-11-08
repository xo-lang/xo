# arm64 emit: wide struct moves made a build cubic in the struct's words

Status: Observed (2026-11-08, GCP aiplatform/v1, Xo Lang 4)

## Symptom

GCP aiplatform/v1's test build spent 11 to 15 minutes in emit (92 to 98%
of the build), while compute/v0.alpha, a larger module, spent about 3.
`--phase-times` showed only that emit was slow. Its heap was normal.

## Cause

G.parallel (compiler/arm64/emit.xo) resolves a call's result moves, a
function's entry moves and its return moves as one parallel move. For each
move it emits, it scanned every pending move against every other one, then
rebuilt the pending list by slicing. That is O(n^3) in the number of words
moved. aiplatform passes and returns wide structs by value, so one move set
holds hundreds of words, and the resolver took 71% of the build's CPU
(XO_PROF=cpu: G.parallel, xo.slice[List[Move]], drop Str, libc memmove).

## Fix

Above 16 moves, parallel_big emits the same moves in the same order in
O(n log n). It gives each location a dense id, keeps counts of pending
readers, and holds candidates in a min-heap of move indices.

To find a slow phase, take a CPU profile of the build (XO_PROF=cpu with a low
XO_PROF_HZ on a long build) before guessing from phase times. Compare against
a module that builds fast.

Loops in emit that grow faster than linear (a rescan per item, or a list
rebuilt by slicing) are invisible on the self build and the goldens,
because their move sets and lines are short. A generated module with wide
structs or long lines finds them: hollow's scan of a long line was the
first (hollow-scan-long-line-quadratic.md). When writing an emit loop over a
function's moves, words or text, check its cost against n in the thousands,
not against the few words seen in hand-written code.

