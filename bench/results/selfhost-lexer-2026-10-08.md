# Self hosting step 1: the Xo lexer in Xo (2026-10-08)

The first step toward self hosting: `internal/lexer` (with `internal/token`)
ported to Xo as the module `selfhost/lexer/`, a CLI that prints a canonical
token dump, the same printer in Go (`lexer.Dump`, hidden `xo lexdump`), and
a differential test (`internal/selfhost`, `TestSelfhostDifferential`).
Decision 0076. No language or compiler change was needed; every gap below
was worked around in Xo and is recorded instead.

## Result

- **The differential test passes on the whole repository**: all 416 `.xo`
  files (std, examples, bench, every testdata tree, the self hosted lexer
  itself), the 93 code blocks of `spec/core.md` and `spec/std.md`, every
  Unicode scalar value as an invalid character (1,112,064 runes in 4 s with
  `XO_SELFHOST_RUNE_STEP=1`; every 13th by default), and a seeded corpus of
  3,000 mutated inputs (300 under `-short`). Byte for byte identical
  tokens, positions (line, column, offset), values, string parts, nested
  `lex_range` tokens of every interpolation, comments, and diagnostics
  (same codes, spans, and messages). 90,000 further mutated inputs under
  seeds 1, 2, and 3 (`XO_SELFHOST_SEED`, `XO_SELFHOST_FUZZ=30000`) also
  matched.
- The mutated corpus reaches all nine lexical codes (XO0150 to XO0158) and
  every message form, including `%q` of invalid characters in each
  `strconv.QuoteRune` shape (`'@'`, `'\\'`, `'\x7f'`, `' '`,
  `'\U000e0001'`, `'é'`, `'\xef\xbf\xbd'` for invalid UTF-8) and `\%c` of a
  byte above 0x7f in an unknown escape.
- It matched on the first full run except for one transcription slip in a
  message (`invalid suffix "x" on number literal`), which the spec examples
  and the fuzz corpus caught at once. No faithful port bug survived to the
  fuzz corpus.
- `xo test selfhost/lexer`: 20 tests (the cases of `internal/lexer`'s tests
  plus 6 property tests), all pass.

## Size

Code lines (non blank, non comment) and physical lines.

| Part | Go | Xo |
|---|---|---|
| Lexer (`lexer.go` / `lexer.xo`) | 923 (1025) | 1060 (1212) |
| Tokens (`token.go` / `token.xo`) | 247 (301) | 167 (198) |
| Library stand ins (`text.xo`: UTF-8, QuoteRune, TrimSpace) | 0 (std) | 167 (211) |
| **Lexer total** | **1170** | **1394 (1.19x)** |
| Dump printer (`dump.go` / `dump.xo`) | 99 | 66 |
| CLI (`cmd/xo/lexdump.go` / `main.xo`) | 59 | 36 |
| Tests (`lexer_test.go` / `lexer_test.xo`) | 274 | 298 |
| Generated IsPrint table (`isprint.xo`) | 0 (strconv) | 1422 |

The Xo lexer is 19 percent longer. About 100 of the extra lines are Bytes
helpers that Go's `strings` package provides (split, join, prefix tests,
CR LF replacement), about 50 are named byte constants (gap 2), and the
operator `match` is longer than Go's `switch` because `xo fmt` puts each
`if`/`else` chain on its own lines.

## Speed

Lexing only (no dump), the whole repository (416 files, 1.06 MB) five
times in one process, files read beforehand; best of 3 interleaved runs on
the shared development host (darwin/arm64, M4), so small differences are
noise (`bench-host-timing-drift.md`).

| Lexer | Time (5 x 1.06 MB) | Throughput | vs Go |
|---|---|---|---|
| Go (`xo lexdump --bench=5`) | 93 ms | 57 MB/s | 1.00x |
| Xo, Go backend (`selfhost/lexer --bench=5`) | 217 ms | 24 MB/s | 2.33x slower |
| Xo, native arm64 | does not build (gap 3) | | |
| Xo, LLVM | does not build (gap 3) | | |

Where the Go backend time goes, read from the generated Go: every method
with a value receiver (`l.peek(n)`, `l.cur()`, `l.pos()`) copies the whole
10 field `Lexer` struct, including three List headers; every widening
`Int(b[i])` is a generic `rt.Conv[int64, uint8]` call with a fault
location; every `+ 1` is a checked `rt.AddI` with a location string.

## Gaps, ranked by impact

1. **No byte string with a builder.** A Go `string` holds any bytes; an Xo
   `Str` must be UTF-8 (`Bytes.to_str()` is `None` otherwise), so every
   token text, value, comment, and message is `Bytes`, and Bytes has no
   builder, no push, and no way to make a byte from an Int. Cooked strings
   are built by `out = out + raw[a..b]` (quadratic on long escaped
   strings), single bytes go through `Bytes.from_hex(hex2(c))`, and
   messages are `"...".bytes() + quote(x) + "...".bytes()` instead of
   interpolation. Fix: a `BytesBuilder` (`write(Bytes)`, `write_byte(UInt8)`,
   `write_str(Str)`, `freeze() -> Bytes`, move only like `StrBuilder`),
   `Bytes.from_list(List[UInt8])`, and `${}` interpolation of `Bytes` into
   a `b"..."` literal.
2. **No numeric character literals.** `Rune` does not convert to `Int`
   (`XO0409`), `Rune(n)` does not exist (`XO0407`), and there is no byte
   literal, so the lexer names 50 byte constants (`C_QUOTE = 0x22`) and the
   operator table matches on `0x2b =>`. Fix: allow `Int(r)` and
   `UInt8(r)` (checked) for `Rune`, add `Rune.try(n: Int) -> Rune?`, and
   let a rune literal in a pattern or comparison against an integer type
   mean its code point (`c == '"'` with `c: Int`), as Go does.
3. **The native backends cannot build it.** `xo build --backend=arm64` (and
   `llvm`, the same SSA builder) rejects the module with 274 errors: no
   `Bytes`, `UInt8`, `UInt64`, `StrBuilder`, `Range` outside `for`,
   `Fs.read`, `Env.args`, `Proc.exit`, `Int.parse`, `Str.starts_with`, or
   `main` with `!`. So the lexer cannot be measured natively, and a self
   hosted compiler cannot be native yet. Next experiment: lower `Bytes` in
   `internal/ssa` as the `Str` representation with a byte index op
   (`BytesAt`), `UInt8`/`UInt64` as unsigned I8/I64, then run
   `xo build --backend=arm64 selfhost/lexer` and burn down the remaining
   list; a lexer only `main` that lexes an embedded `const` source would
   measure the core before `Fs` and `Env` are lowered.
4. **No Unicode and quoting functions in std.** Matching Go's messages
   needed `utf8.DecodeRune`, `utf8.RuneCount`, `utf8.AppendRune`,
   `strconv.QuoteRune` with `strconv.IsPrint`, and `unicode.IsSpace`, all
   written in `text.xo` (167 lines), plus a 710 range IsPrint table
   generated from Go (`isprint.xo`, kept current by `TestIsPrintTable`).
   Fix: `Bytes.decode_rune(i) -> (Rune, Int)`, `Rune.is_print()`,
   `Rune.quote() -> Str`, `Str.quote() -> Str` (Go `%q`), and
   `Rune.encode() -> Bytes` in the prelude.
5. **XO0305 on a large enum.** `Kind` has 88 variants. Any `match` with a
   `_` arm (such as "these 8 kinds have short names, the rest none") warns
   and asks to list the other 79, printed on one line. Workarounds used: an
   `is` or-pattern (`k is Ident | Int | ...`) and a lookup list of
   `(Kind, Str)` tuples. Fix: do not warn when the `_` arm hides more than
   half of the enum's variants (it is then clearly a default), and cap the
   hidden variant list in the message ("and 70 more").
6. **Bytes lacks the Str methods.** No `find`, `starts_with`, `contains`,
   `split`, `replace`, `trim`, or `index_of` on `Bytes`: about 100 lines of
   helpers (`slashes`, `has_double_slash`, `index_rbrace`, `replace_crlf`,
   `split_byte`, `join_byte`, `is_space_tab`, `leading_space_tab`). Fix:
   give `Bytes` the byte level `Str` methods (std.md 2.2).
7. **No local helper that mutates enclosing state.** Go's `stringLit` uses
   a `flush` closure and `cook` an `errAt` closure; Xo closures cannot
   mutate captured `var`s (`XO0502`), so both are inlined and the state
   (`acc`, `seg`, `cur_start`) is reset by hand at three places. Fix:
   allow a non escaping local closure (called, never stored or passed) to
   mutate captured `var`s, which keeps value semantics.
8. **Go backend speed (2.33x of Go).** Value receivers of large structs are
   copied per call; widening integer conversions are generic checked
   calls; checked adds carry location strings. Fix in `internal/gen`: pass
   non `var` struct receivers as pointers (they cannot be mutated), emit
   widening conversions (`UInt8` to `Int`) as plain Go conversions, and
   let range analysis prove `off + 1` cannot overflow when `off < end`.
9. **`xo fmt` and tables.** A `const` list keeps one element per line, so
   the 1420 numbers of the IsPrint table are 1420 lines (the generator
   emits that layout so `xo fmt --check` passes). Fix: keep a source
   layout of several elements per line when every line has the same count
   (a table).
10. **No module boundary between the library and the CLI.** The lexer and
    its `main` share one module (a module with `fn main` is a program), so
    another module cannot `use` the lexer without also getting a `main`.
    For the parser port this needs either a `selfhost/xolex` program module
    using `selfhost/lexer` through an `xo.mod`, or a rule that a library
    module's `main.xo` is ignored when it is imported.

Things that worked well and need no change: `derive Display` on the 88
variant enum prints the Go constant names directly; a `const` Map of enum
values for keywords; `k is A | B | ...` on enums; tuple returns with
`let (a, b) = ...`; early `return` from an `if` expression arm; `UInt64`
arithmetic with typed constants for `strconv.ParseUint`'s overflow rules;
property tests over `List[Int]` turned into byte strings.

## Diagnostics that confused

- `XO0407: \`Rune\` is a type and cannot be called` for `Rune(n)`, when
  `Int(x)` and `Float(x)` are written the same way. It should say that
  `Rune` has no conversion from `Int`, and point at the alternative (there
  is none today, gap 2).
- `XO0409: cannot convert \`Rune\` to \`Int\`; numeric conversions take a
  number` is accurate but has no note on how to get a code point.
- `XO0305` printed 79 variant names on one line (gap 5).
- The arm64 build printed "the SSA builder does not support this
  expression" 91 times without naming the expression kind; the useful
  messages were the ones naming a type or method.
- Good: a property test that faulted on `n.abs()` for `Int.MIN` reported
  the case, the input, and the fault location at once
  (`gotchas/property-int-min-abs.md`).

## How to reproduce

```
go test ./internal/selfhost                                       # differential (repo, spec, runes, 3000 fuzz) and xo test selfhost/lexer
XO_SELFHOST_SEED=2 XO_SELFHOST_FUZZ=30000 go test ./internal/selfhost -run TestSelfhostDifferential
XO_SELFHOST_RUNE_STEP=1 go test ./internal/selfhost -run TestSelfhostDifferential/runes
go run ./cmd/xo lexdump file.xo                                   # Go lexer dump
go run ./cmd/xo run selfhost/lexer -- file.xo                     # Xo lexer dump (same text)
go run ./cmd/xo lexdump --bench=5 $(find . -name '*.xo' -not -path './.*')
go run ./cmd/xo build -o /tmp/xolex selfhost/lexer && /tmp/xolex --bench=5 $(find . -name '*.xo' -not -path './.*')
```
