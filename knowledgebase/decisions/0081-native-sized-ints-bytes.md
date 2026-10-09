# 0081: Sized integers, Rune, and Bytes on the native backends

Status: Accepted (2026-10-08). Implemented in `internal/ssa`
(`build_int.go`, `build_expr.go`, `build_io.go`, `ssa.go`),
`internal/llvm/fromssa.go`, `internal/arm64/isel.go` and `ranges.go`,
`std/llrt/xo_bytes.c`, `std/native/prelude.xo` and `rt.xo`. Gap 3 of
`bench/results/selfhost-lexer-2026-10-08.md`. Evidence: the dated section
of that file (the self hosted lexer builds natively and matches the Go
lexer byte for byte on every input of the differential test).

## Context
The shared SSA builder (decision 0045) lowered Int, Float, Bool, and Str
only; `selfhost/lexer` hit 274 build errors on the arm64 and LLVM
backends: no `UInt8` (`b[i]`), `UInt64` (`strconv.ParseUint` rules),
Bytes indexing and slicing, `StrBuilder`, and no CLI capability methods.
A self hosted toolchain that cannot be built natively cannot replace the
Go one.

## Decision
- **Sized integers live in the Int word.** `Int8` to `UInt64` and `Rune`
  are `KInt` SSA types with `Bits` and `Unsigned` (interned by name), held
  sign or zero extended in 64 bits; a `UInt64` above the Int range is a
  negative word. So everything that moves, stores, compares narrow values,
  or does `&`, `|`, `^` is the Int code every backend already has, and
  the analyses of the arm64 backend stay sound (they see i64 operations
  that are exactly what runs).
- **What can leave the range is a runtime call.** `+ - * / % << >>` and
  unary minus on a sized type call `rt.int_op` (op, operands, bits,
  signedness, location), which checks and faults with std/rt's messages
  (`integer overflow: 200 + 56 does not fit UInt8`); conversions that can
  fail call `rt.int_conv` or `rt.rune_of`; widening conversions are the
  new `OpIntConv` (no code). `~` on an unsigned narrow type is `^` with
  its mask. A UInt64 is ordered by flipping the sign bit of both
  operands; its text, its Float, and its arithmetic are C. Sized
  arithmetic is rare in byte level code (comparisons dominate), so calls
  cost little; inline checks can come later per operation.
- **Rune literals** where an integer is expected (decision 0079) are
  integer constants; a `Rune` displays as its character
  (`rs_append_rune`) and its Debug text quotes it.
- **Bytes** stays the Str representation (decision 0069). `b[i]` is a new
  runtime call `bytes.index` lowered inline by both backends (bounds check,
  byte load); `b"..."` is a string constant; `s[a..b]` on Str (with the
  UTF-8 boundary check), Bytes, and List (a generated copy loop) fault as
  std/rt does.
- **Builders are their text.** `StrBuilder` and `BytesBuilder` lower to a
  Str; `write` is `str.concat`, which appends in place when the builder
  (move only) holds the only reference, `write_byte`/`write_rune` are
  `rs_push_*`; `freeze` is the value.
- **CLI capabilities**: `os.fs.read` (status and FsErr built like NetErr),
  `os.env.args`, `os.proc.exit`, with `Fs` a capability. The Rune, Str,
  and Bytes text methods of decision 0079 are in `std/native/prelude.xo`
  over C intrinsics with Unicode tables generated from Go
  (`xo_unicode.c`, checked by `TestUnicodeTables`).
- **Constants of collection types are built once.** A `const` List, Map,
  or struct read in a function was rebuilt at every read (the lexer's
  keyword Map once per identifier: 60 percent of its time). An entry
  function now builds it on first use (`rt.const_once`), marked shared and
  kept immortal in a runtime slot; at exit under `XO_LEAKCHECK` the slots
  are released, so leak checks stay exact.

## Consequences
- `xo build --backend=arm64` and `--backend=llvm` build
  `selfhost/lexer`; `TestSelfhostDifferential` runs on all three backends
  (repository, spec blocks, all runes, mutated inputs) and they match the
  Go lexer byte for byte.
- New goldens in `internal/llvm/testdata/golden` (`native_sized_ints`,
  `native_bytes`, and four fault programs) run on both native backends
  with leak and share checks.
- `xo test` still has no native backend: the lexer's Xo tests run on the
  Go backend only.
- Unsupported constructs now name their AST kind ("this expression
  (WithExpr)") instead of "this expression".

## Alternatives
- **Real narrow machine types in SSA (i8, i16, i32 kinds)**: smaller
  lists of bytes and native overflow flags, but every backend, every
  analysis, and the RC and equality helpers would need each new kind;
  the word representation needed none of that to be correct first.
- **Lower unsigned ordering with unsigned compares in each backend**:
  one less instruction, but the arm64 range analysis reasons about
  compares as signed; the sign flip keeps it sound with no change there.
- **Rebuild constants at each read** (as before): simplest, but turned a
  Map lookup into a Map construction in the lexer's hottest path.
