# A parse list from another shape of check is prefetched in full

Status: Observed (2026-11-11, Xo Lang 2)

## Symptom

The first edit of a std module's tests (`xo test --std-module=native/json`
after an unedited run) peaked at 218 MB on linux/arm64 (0.48.0), the
later edits at 115 to 119 MB, with the same `cache` lines in every edit
(std's variant reused, 31 modules from source, `cache stdx used`,
`cache parse 13/13 reused`). Its check held 154 MB of heap against 67 MB,
and took 79 ms against 45 ms. base64's first edit: 199 MB against 97 to
100.

## Cause

The parse prefetch (decision 0220 addendum stage 2) parses on tasks every
file in the build target's parse list, the files the target's last check
parsed. The unedited build declared std's runtime from source (its 173
`std/native/llrt` files, 194 of the list's 216 entries in full), and an
edit build takes std's declarations from their record and parses 11 to 13
files. The first edit read the old list and parsed all 216, about 95 MB of
trees no check asked for (a heap profile at the end of check, edit one
minus edit two: +95 MB under `parser.parse_file` in `ast_cached_parse`).
The `cache parse H/N` line counts only the check's own parses, so it could
not show the waste. A first guess was wrong: re-decoding each stored entry
to drop the full trees made it worse (178 MB), since the decoded entries
of those files hold their bodies too.

## Fix

compiler/xo/bcache.xo parse_shape: the list's first line records the
check's shape (`k rec=BOOL src=MODULES`), taken before the check (which
consumes std's record). A list of another shape prefetches only the
program's own files. A list without the line (an older xo's) is
prefetched as before. `--phase-times` prints `cache prefetch N files`, and
internal/arm64 TestSepcompStdModuleEdits fails when the prefetch reads
more files than the check parses (it read 216 for 11 without the fix).

## How to avoid

A cache that predicts the next build from the last one (a list, a
prefetch, a memo) records what the last build's shape was, and its
counters count what was fetched, not only what was used.
