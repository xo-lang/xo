# std/charset changes

## 2026-11-16: perf: decode builds its Str without a second UTF-8 check

- Kind: perf
- Commit: this push
- Found by: task #88, the w1252_decode row of bench/perf/charset over 1.05 (a profile: the to_str check was 28% of the decode)
- Go reference: golang.org/x/text/encoding/charmap and unicode decoders (they return the bytes they wrote, unchecked)
- Test: internal/stdtest/testdata/corpus/charset and std/charset_test.xo (decode of every byte and of invalid UTF-16 against x/text)
- Timing: bench/perf/charset w1252_decode 1.27 of Go before, 0.87 to 0.93 after (Mac, three runs); the other rows 0.61 to 0.83 (host 1 row to follow)

`decode` wrote every rune it made as its UTF-8 encoding (U+FFFD for
what is invalid) and then checked the result as UTF-8 again. It now
takes the bytes as they are. The results are the same.

## 2026-11-15: feature: UTF-8 steps from the end and stream checks (Go's utf8 DecodeLastRune, DecodeLastRuneInString, FullRune, FullRuneInString, RuneStart)

- Kind: feature
- Commit: this push
- Found by: the stdaudit features line (decision 0655)
- Go reference: unicode/utf8 utf8.go (go1.26.1)
- Test: internal/stdtest/testdata/corpus/charset (scripts/stdcorpus/charsetoracle): 769 byte strings (utf8_test.go's utf8map runes cut from both ends, its invalid sequences, 600 seeded random strings over lead, continuation and invalid bytes) with Go's DecodeLastRune and FullRune, the Str forms where the bytes are UTF-8; RuneStart of every byte (arm64, LLVM, Go)
- Timing: none

`utf8_decode_last_rune` reads the last rune of a buffer, `utf8_full_rune`
says whether a buffer starts with a whole sequence (for decoding a
stream in pieces), and `utf8_rune_start` whether a byte can start one,
each as Go's unicode/utf8 does.

## 2026-11-15: feature: one-rune UTF-16 steps and REPLACEMENT_CHAR (Go's utf16 AppendRune, DecodeRune, EncodeRune, IsSurrogate, RuneLen; unicode.ReplacementChar)

- Kind: feature
- Commit: this push
- Found by: scripts/stdaudit gaps utf16.AppendRune, DecodeRune, EncodeRune, IsSurrogate, RuneLen and unicode.ReplacementChar (decision 0655)
- Go reference: unicode/utf16 utf16.go and utf16_test.go (go1.26.11; Copyright 2010 The Go Authors, BSD-style license)
- Test: std/charset_test.xo, Go's TestRuneLen, TestAppendRune, TestEncodeRune, TestDecodeRune and TestIsSurrogate tables (arm64, llvm; corpus charset on go)
- Shipped in: not yet released

The functions take code points and units as Int, as Go's take rune and uint16 values, so a surrogate, -1 and 0x110000 behave as in Go.

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
