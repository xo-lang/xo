# 0101: Native Float32, Secret, Buf, order, and other value gaps

Status: Accepted (2026-10-09). Implemented in `internal/ssa` (build.go
`typ`, build_call.go, build_expr.go, build_map.go `cmpFunc`,
`listConcat`, build_show.go), `std/native/prelude.xo`, and
`std/llrt/xo_bytes.c`. Goldens: `internal/llvm/testdata/golden/float32`,
`native_gaps`, and the internal/gen goldens listed in
`internal/llvm/testdata/gen_native.txt` (TestGenGoldensNative, which
compares Go, LLVM, and arm64 on every listed program).

## Context

The spec audit (`bench/results/spec-audit-2026-10-09.md`) listed as
native `XO1201` gaps: Float32, `Secret[T]`, `Buf[T]`, std calls whose
defaults only the std declaration has, `get_or_fault` on keys other than
Str and Int, `Int32.try` and the other sized `try`s, `Int.pow`,
`clamp`, `<` on `derive Ord` types, Set algebra, `Map.filter`,
`List.insert`, `List.remove_at`, `indexed()` outside a `for`, `+` on
lists, and builtin collection methods called through an interface or a
constraint.

## Decision

1. **Float32 is a double holding a float32 value** (`Module.Float32`, a
   `KFloat` with `Bits` 32). Literals and conversions round through
   `float` (`rt.f32_round`, `rt.i64_to_f32`), and `+ - * /` round the
   double result, which is the correctly rounded float32 result (a
   double has more than twice float32's precision). Text, comparisons,
   and `Int(x)` are the double's, as on the Go backend, which formats a
   Float32 as `FloatStr(float64(x))`. No backend change; a call per
   operation (a later backend can lower `fptrunc`/`fpext` inline).
2. **`Secret[T]`** is a struct holding the value; Display, Debug (in
   structs, lists, maps, `Map[Str, Debug]` log fields) write
   `<redacted>`; `reveal` reads the field.
3. **`Buf[T]`** is its List (as StrBuilder is its Str): `push` appends
   in place while the move only buffer holds the only reference,
   `freeze` is the List, Debug text `Buf[1, 2]`.
4. **Order**: `<`, `<=`, `>`, `>=` on structs, enums, tuples, and lists
   call `xo.cmp[T]`, generated per type with std/rt's `Cmp`: fields in
   order, variants by declaration order then fields, shorter list first,
   Float NaN after every number, false before true.
5. **Interface values of Debug** (`xo.dyn`) carry a third field, the
   Debug text when it differs from the text (a Str is quoted, a type
   with a `display` method shows its fields), so `.debug()` of a
   `Map[Str, Debug]` matches the Go backend while logs keep the Display
   text.
6. **`==` on values holding a function or a runtime handle** outside an
   interface value is XO1201: the backends' equality helpers do not
   compare them, and the check used to accept such programs (the
   internal/gen golden `http_scoped` compares two requests holding
   request values and failed in the LLVM lowering). Since 2026-10-10
   `http.Request` compares its six spec fields natively (decision 0112's
   note), so `http_scoped` builds with `--no-gc`.
7. The rest are native prelude functions in Xo (`map_filter`,
   `set_union`, `set_intersect`, `set_difference`, `list_insert`,
   `list_remove_at`, `list_indexed`, `int_pow`, with std/rt's fault
   texts at the caller's line) or builder lowerings (`T.try`, `clamp`,
   `min` and `max` on Str, `xs + ys`, a std declaration's default when
   the native implementation has none, the key's Debug text in
   `get_or_fault`'s fault).

## Consequences

- 66 internal/gen goldens build natively with output identical to the
  Go backend, checked on every run of the internal/llvm tests.
- Float32 arithmetic costs a runtime call per operation natively.
- An or pattern and shorthand field patterns (`Bag{items}`,
  `Node(left, ..)`) now bind natively (they silently bound nothing or
  the wrong alternative, gotchas/native-or-pattern-bindings.md).

## Alternatives

- **Float32 as an LLVM `float` and arm64 `s` registers**: exact and
  faster, but a new value class in both backends' layouts, calling
  conventions, and runtime calls; the double form is exact for every
  operation Xo has on Float32.
- **Secret as its value** (no struct): Display and Debug would need the
  checker type at every output path; the struct makes it a distinct SSA
  type.
