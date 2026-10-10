# Self hosted parser, 2026-10-10

Decision 0127. The Xo parser (`selfhost/parser`, 4,272 lines of Xo
with its dump, CLI, and tests) against internal/parser (Go).

## Coverage

The whole grammar of core 15 as internal/parser implements it:
`use` (plain, `go`, `c` with `link` and `uses`), every declaration
(functions and methods with receivers, type parameters with constraints
and `Effects`, defaults, `self`, error specs, `uses`/`requires`/`ensures`
clauses, struct fields with defaults, `invariant`, `derive`, enum
variants with named and `from` fields, interfaces, aliases, newtypes,
constants, tests with `for`, `seed`, `uses`), doc comments, every type
form (qualified, generic, optional, tuple, function types with effects
and the trailing `uses` rule), every statement (labels, `defer` of a
block or statement, local `const`, compound assignment), expressions with
the precedence of core 15 (`is`, ranges, `??` right associative, open
ranges inside indexes, type arguments vs indexing, `{` map vs block,
struct literals off in condition heads, interpolation re lexed and
parsed, `with`, closures, `if let`, `match` with guards, `select` with
`after` and bindings, `scope`, `while` as an expression), all patterns,
and signature mode. Go's `for k, v := range x` is kept (`go_range`) for the
checker as the Go parser does. Every habit of decision 0087 is a
rejection.

Simpler than the Go parser: the first error stops the parse and is the
only diagnostic (code and span, shorter messages); no recovery, no fixes.

## Agreement

`go test ./internal/selfhost -run TestSelfhostParserDifferential`, all
three backends (go, arm64, llvm):

| input | files | dumps identical (both accept) | both reject |
|---|---|---|---|
| repository .xo files (std excluded) | 815 | 708 | 107 (habit and error fixtures) |
| std/*.xo, signature mode | 28 | 28 | 0 |
| spec code blocks | 106 | 31 | 75 (fragments) |
| progen programs (decision 0110) | 150 | 150 | 0 |
| token mutations (seed 20261010) | 2,000 | 216 | 1,784 |

Disagreements: none. Extra runs on the go backend before the commit:
30,000 mutated inputs (seeds 1 to 3, 10,000 each, about 6.6% accepted)
and 1,000 generated programs, no disagreement.

## Speed

Five largest real files (selfhost/parser/parser.xo, selfhost/lexer/lexer.xo,
selfhost/parser/expr.xo, selfhost/parser/dump.xo,
selfhost/lexer/lexer_test.xo: 133,790 bytes), each parsed 200 times
(26.8 MB), `--bench=200`, Apple M-series host, median of 3 runs; the time
includes lexing on both sides.

| parser | ms | vs Go |
|---|---|---|
| internal/parser (Go, `xo astdump --bench`) | 1,580 | 1.00 |
| selfhost/parser, Go backend | 4,044 | 2.56x slower |
| selfhost/parser, arm64 `--no-gc` | 1,790 | 1.13x slower |
| selfhost/parser, LLVM `--no-gc` | 1,220 | 1.30x faster |

Native builds without `--no-gc` measured the same within 1 percent. The Go
lexer alone takes 613 ms of the Go parser's 1,580 on this input.

## Gaps found

1. **Fixed: arm64 frames over 4000 bytes** were refused
   (`Parser.parse_file`, 5120 bytes) and **helpers of 80+ word values**
   wrote `stp x29, x30, [sp, #688]`, out of stp's reach. Decision 0128:
   frames to 28 KiB with page probes, two `str` past 504 bytes. Run
   programs `big_frame`, `wide_values`.
2. **Open: arm64 spill slots are never reused.** The frame of
   `parse_file` is 640 slots because every spilled value gets its own.
   Next step: slot coloring by live range in `internal/arm64/regalloc.go`
   (measure with `TestBigFrameProbes`, which should then need fewer
   probes).
3. **Open, small: keywords cannot be field names** (`uses`, `invariant`):
   the AST fields are `effects`, `inv`, `imports`, and the dump maps them
   back to Go's names. Same as Go; noted, no change proposed.
4. **Open: no Xo to Go library calls**, so the comparison runs the Xo
   parser as a process over files, as for the lexer (decision 0076).

## Next steps

- Port the Go parser's diagnostics (codes, messages, recovery, habit
  fixes) and compare `diag` lines too; then the mutation corpus checks
  error paths byte for byte.
- Spill slot reuse on arm64 (gap 2).
- The self hosted checker on top of `selfhost/parser`'s typed tree.
