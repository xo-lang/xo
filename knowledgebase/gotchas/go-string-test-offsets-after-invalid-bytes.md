# Go's string test offsets move after an invalid byte

Status: Observed (2026-10-23), in Str.find_any's corpus port.

**Symptom:** porting Go's `lastIndexAnyTests` into
`scripts/stdcorpus/strsearch.go`, the case `{"012\x80bcb\x80210", "\xffb", 7}`
failed the generation-time check with `LastIndexAny = 9`, though Go's
table says 7.

**Cause:** Go's tables count bytes of a raw string, where `\x80` is one
byte. An Xo `Str` reads an invalid byte as U+FFFD (decision 0259), three
bytes, so every offset after it moves by two per invalid byte. The corpus
compares Xo against Go run on the *converted* text (`textOf`), so Go's
recorded offset is not the expectation there. Only the offsets after an
invalid byte change; a match on the invalid byte itself keeps its offset
if everything before it is ASCII.

**Fix:** assert Go's table value against Go on the raw input (the faithful
port), compute the corpus's expectation with Go on `textOf(input)`, and
print a note when the two differ (strsearch.go `goAnyCases`). Two of Go's
cases contain invalid bytes; one moves (7 to 9). The same applies to any
Go string test whose expectations are byte offsets: `strings.Index`,
`LastIndex`, `IndexAny`, `LastIndexAny`, `IndexFunc`.
