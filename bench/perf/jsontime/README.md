# jsontime

std/json decode of a `List[Time]` (2000 RFC 3339 strings, 200 rounds), the
workload of the 2026-10-24 change that parses a Time from the tape's bytes.
`xo bench --ab A B` with A built by the xo before the change and B after.
Result: wall 0.898 (0.885 to 0.915), instructions 1.015; see
std/changes/json.md.
