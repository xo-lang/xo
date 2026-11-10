# std/charset changes

## 2026-11-10: refactor: one file with its bodies (decision 0588 stage 2)

- Kind: refactor
- Commit: this commit
- Found by: decision 0588 stage 2 (pure modules merged with their implementation)
- Go reference: none (no behavior change)
- Test: internal/stdtest corpus charset on go, arm64 and llvm

std/charset.xo now holds the bodies of std/native/charset.xo (removed), and the Go
backend compiles them from it as its own package; std/rt/charset.go (the Go backend's display methods) is removed, the
module's own `display` serves it. The API, texts and errors
are unchanged: every item the declaration file had is `pub`, with the same
parameter names.

## 2026-11-03: feature: std/charset, UTF-16, Latin-1 and Windows-1252 decode and encode

- Kind: feature
- Commit: this commit
- Found by: the 2026-10-30 backend survey (std-coverage.md rank 4: Excel's Unicode Text and legacy Windows-1252 exports); decision 0470
- Go reference: golang.org/x/text v0.43.0 encoding/unicode (UTF16) and charmap (ISO8859_1, Windows1252), htmlindex labels, Go's unicode/utf16
- Test: internal/stdtest/testdata/corpus/charset (x/text's decoders and encoders on hand picked and seeded random bytes and texts, Go's utf16.Encode and Decode, htmlindex's labels) on arm64, LLVM and the Go backend; bench/perf/charset twincheck
- Timing: see bench/results/std-twin-rows9-2026-11-03.md

`charset.decode`, `encode`, `lookup`, `utf16_units` and `from_utf16_units` (spec/std.md 54). One source (std/native/charset.xo) for the native backends and, through std/go_compiled.txt, the Go backend. Differs from x/text where the Encoding Standard does: the five unassigned Windows-1252 bytes map to U+0081 and friends in both directions; and a lone low UTF-16 surrogate gets its own U+FFFD, as in utf16.Decode.
