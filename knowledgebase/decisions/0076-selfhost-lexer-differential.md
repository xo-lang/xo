# 0076: Self hosting starts with the lexer, checked by a byte for byte differential test

Status: Accepted (2026-10-08). Implemented in `selfhost/lexer/` (Xo),
`internal/lexer/dump.go` (`Dump`), `cmd/xo/lexdump.go` (hidden
`xo lexdump`), and `internal/selfhost` (`TestSelfhostDifferential`,
`TestSelfhostXoTests`, `TestIsPrintTable`). Evidence:
`bench/results/selfhost-lexer-2026-10-08.md`.

## Context
Writing the compiler in Xo is the strongest test of whether Xo is good for
the kind of program its own toolchain is, and it surfaces language and std
gaps with real code instead of benchmarks. It has to happen in steps, each
proved equal to the Go implementation, or the two drift. The lexer is the
first step: small, pure, and with a rich error surface.

## Decision
- **Port faithfully, gaps recorded, no language changes for the port.** The
  Xo lexer reproduces `internal/lexer` exactly: every token kind,
  positions (line, column, byte offset), values, string parts and
  `lex_range` of interpolations, comments, and diagnostics with the same
  codes, spans, and messages (including Go's `%q` of a rune, via a port of
  `strconv.QuoteRune` and a generated IsPrint table). Language and std gaps
  found on the way go in the results file, ranked, each with a proposed
  fix; they are fixed by their own decisions.
- **Text is `Bytes`.** A Go `string` holds any bytes; Xo `Str` must be
  UTF-8. Token text, values, comments, and messages are `Bytes` so files
  that are not valid UTF-8 lex the same.
- **One canonical dump, two printers.** `lexer.Dump` (Go) and `dump.xo`
  (Xo) print the same line format (kind, span with offsets, quoted text and
  value, parts with nested range tokens, comments, then diagnostics in
  report order), quoting byte by byte (`\xHH` outside printable ASCII) so
  invalid UTF-8 compares exactly. `xo lexdump` (hidden, not in the usage
  text) prints the Go side; `selfhost/lexer` is the Xo CLI.
- **Differential test in Go.** `internal/selfhost` builds the Xo lexer with
  `internal/build` and requires identical output on every `.xo` file of the
  repository, the spec's code blocks, every Unicode scalar value (sampled
  by default), and a seeded mutation corpus (3000 inputs, 300 under
  `-short`; `XO_SELFHOST_SEED`, `XO_SELFHOST_FUZZ`, `XO_SELFHOST_KEEP`).
  It runs under `-short` too, since it is the only guard against drift.
- **Layout.** `selfhost/<component>/` holds each ported component as an
  Xo module; the lexer module also holds its CLI (`main.xo`).

## Consequences
- Any change to `internal/lexer` (a new token, a message) must be made in
  `selfhost/lexer` in the same change, or `go test -short ./...` fails.
  The same holds for a new lexical diagnostic code.
- The test costs one Xo build (about 3 s cold, cached afterwards) and about
  1 s of lexing.
- The Xo lexer builds only on the Go backend: the native backends lack
  `Bytes`, `UInt8`, `UInt64`, and the capability methods it uses (gap 3 in
  the results).

## Alternatives
- **Compare token streams in memory** (call the Xo lexer from Go): there is
  no Xo to Go library export; a process boundary with a text format also
  tests the CLI path a self hosted compiler would use.
- **Compare against `xo parse --json`**: diagnostics there pass through the
  parser, which reports and suppresses its own; the lexer surface would be
  only partly visible.
- **Use `Str` for text and skip invalid UTF-8 inputs**: simpler, but the
  Go lexer is defined on bytes and the fuzz corpus would have to avoid the
  inputs most likely to differ.
