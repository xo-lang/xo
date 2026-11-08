# head cuts a golden diff and fakes a truncated run

Status: Observed (2026-11-08)

## Symptom

`go test ./internal/arm64 -run 'TestGoldenRun/pack_lists$' | head -30`
showed the "got" output ending seven lines in, after
`rev 45960 7 eq true false true`, with exit 0: it read as a program that
stopped printing early, a silent miscompile, and was reported as a
0.44.0 blocker with a bisect assigned.

## Cause

A golden failure prints the whole want block first and the got block
after it, so `head` cut the got block, not the program. The real
difference was one line at the end (a stale want.txt after
dffd08f21 changed how a Float32 list prints in JSON).

## Fix

Never cut a golden failure with `head`. grep the lines around the
difference (`grep -A3 -E '^ +(f32|--- got)'`), or write the got text to
a file and `diff` it against want.txt. Before calling an output
truncated, read the whole got block and its exit line.
