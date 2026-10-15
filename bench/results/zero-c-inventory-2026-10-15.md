# xo_rt.c inventory after the generated-code entries (2026-10-15)

What keeps each section of std/llrt/xo_rt.c (main at e2d5762d, 3,209
lines) now that generated code calls Xo for every entry but the ones by
design, and what moving its root caller to Xo would free. Two halves by
line: the first (lines 1 to 1,698, Memory through the worker pool) by
Xo Lang 3, the second (1,699 to 3,209, scopes and tasks through Entry) by
Xo Lang 2 (xo-4b); the ranked merge is below them.

Columns: lines; what keeps it (called by current generated code; called
only by other C, with the root caller; Windows only; seed only, gone
after the next seed; C by design, with the reason); the lines freed if
its root caller moved to Xo; a rough effort (S under an hour, M a few
hours, L a day or more).

## Ranked (lines freed per unit of effort)

(merged once both halves are in)

## First half (Xo Lang 3)

(in progress)

## Second half (xo-4b)

(in progress)
