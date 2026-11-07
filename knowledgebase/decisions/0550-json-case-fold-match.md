# 0550: Case-insensitive key matching for derive Json

Status: Accepted (owner, 2026-11-07; proposed by Xo Lang 2, 2026-11-07, asked for by xo-lead after the goversioninfo port)

## Context

Go's `encoding/json` matches an object's keys to struct fields without
regard to case: `{"major": 2}` sets a field `Major`. Go programs' config
files rely on it (goversioninfo's documented `versioninfo.json` examples
mix `"Major"` and `"major"`), and so does any Go service whose clients
send keys in another case. `derive Json` matches keys exactly
(decision 0277: a field's key is its name, its `as "key"` name or its
`case:` rule's spelling). A Go port that must read the same files cannot
use the derive: the goversioninfo port decodes `json.parse_value`'s
Value by hand, about 200 lines (its FEEDBACK.md, std finding 3). The
lead expects the same need in aws-19's and the blog's Go config ports.

Go's rule, verified with go1.26.1 on 2026-11-07 (`json.Unmarshal`):

1. The object's members are applied in document order; each member that
   selects a field assigns it, so for one field the last selecting member
   wins: `{"Major":1,"major":2,"Major":3}` gives `Major = 3`, and
   `{"Major":1,"major":2}` gives 2 (a folded match after an exact one
   still assigns).
2. A member selects the field whose key equals it exactly; if none does,
   the first field in declaration order whose key folds equal to it
   (Go's `foldName`, encoding/json/fold.go). With fields `A json:"Key"`
   and `B json:"KEY"`: `{"key":1}` sets A (the first declared),
   `{"KEY":1}` sets B (exact), `{"kEy":1,"Key":2}` sets A to 2, and
   `{"Key":2,"kEy":1}` sets A to 1; declared the other way round (B
   first), `{"key":1}` sets B.
3. A member that selects no field is ignored, as today.

## Decision (proposed)

- `derive Json(match: fold)` on a struct, or on an enum for its variants'
  fields, makes its decoder follow rules 1 to 3. The option is spelled
  like 0277's `case:`, an identifier value; `match: exact` is the
  default and may be written. It combines with `case:` and `as "key"`:
  those give each field's key, and folding compares against that key.
- Encoding is unchanged: keys are written as declared.
- The option is per type, not inherited: a nested struct folds only when
  it says so too. Go folds everywhere, so a port marks each config type;
  the checker does not require it of field types. No field-level form:
  Go has none, and no port needs a field that matches exactly inside a
  folding type.
- Every backend: native and LLVM (the SSA decoder), wasm (same SSA), and
  the Go backend's runtime.

## Implementation and cost

- Native, LLVM, wasm: `dec_fields` (compiler/ssa/json.xo) finds every
  field's member in one pass with jsontext's `find_all` over packed keys.
  A folding type calls a `find_all_fold` with a second packed list of
  the keys folded at compile time. Per member: the exact comparison as
  today; only on a miss, the member's key is folded (ASCII in place over
  its bytes; a non-ASCII rune through Go's fold rule, ported from
  fold.go with attribution) and compared with the folded keys in
  declaration order. The member index recorded for a field is the last
  one that selected it, which keeps the current duplicate handling
  (earlier values still decode first and are dropped, as Go reads every
  member).
- Cost: zero for types without the option (the runtime call is chosen at
  compile time). For a folding type whose input uses the declared keys,
  every member hits exactly, so the cost is one extra branch per member;
  a member that misses pays one fold of its key and up to n compares.
  Measured before the code is accepted (decision 0340, linux/arm64
  offload host): `xo bench --ab` decoding a 10 field struct (a) with
  exact keys, (b) with every key lowercased, (c) with unknown keys, for
  the same type with and without the option.
- Go backend: generated decoders read `rt.JsonObject`'s
  `map[string]any`, which loses member order. A folding type's decoder
  reads an ordered member list instead (a new `rt.JsonMembers`) and
  applies the same selection, so the Go backend agrees with native.
- Tests: a golden `json_fold` on every backend and in
  TestGoldenGoParity (exact, fold, a non-ASCII fold, two fields folding
  equal in both declaration orders, last match wins, `case:` with fold,
  a nested non-folding struct), and a differential against Go's
  encoding/json over generated objects (the goversioninfo corpus
  generator already varies key case).
- Docs: spec/core.md (derive Json options, next to `case:`),
  spec/std.md 5, std/changes/json.md; an unknown `match:` value reuses
  the derive option error code if its text fits, else a new XO code.

## Implementation (Xo Lang 2, 2026-11-07)

- Spelling: `match` is a keyword, so the parser takes it as a derive
  argument's name in that one position (compiler/parser parse_derive);
  the checker sets the type's `json_fold` (compiler/check decl.xo
  json_match, exported in the std export, version v9).
- Native, LLVM and wasm: `dec_fields_fold` (compiler/ssa/json.xo) calls
  `jsontext.find_all_fold` with two lists packed at compile time, the
  exact keys in find_all's form and the folded keys, each folded key
  marked when an earlier key folds equal to it (it then matches only
  exactly, so the first declared field wins). Per member: the exact
  probe loop as find_all's; on a miss, an unescaped ASCII key (no
  STR_HIGH) compares against the folded keys a word at a time (upper8,
  a-z less 0x20 per byte); an escaped or non-ASCII key goes through a
  cold path (fold_match_slow: the unquoted key, Go's fold rune by rune
  with unicode.to_case, an invalid byte as U+FFFD).
- Go's error order (FEEDBACK item 6, agreed with xo-lead 2026-11-07):
  a folding type's decoder decodes as dec_fields does until any error,
  then jumps to one cold block (dec_fields_fold_err) that decodes every
  selected member again, replaced ones included, and keeps one error: a hard one (a member whose type is a via type, or
  an Option, List, Set or Map of one: Xo's analogue of an UnmarshalJSON
  error, which stops Go's decode and wins) before any type error, else
  the earliest member in document order (its tape index); a missing
  field, which Go does not report, after every member's. A nested type
  reports its own earliest error, so the order holds across the
  document. Types without the option keep today's order (declaration
  order, first error returned).
- Go backend: `rt.JsonObjectFold` selects the members over the object's
  ordered members (jsonObj.all, else its keys) with encoding/json's
  foldName ported from fold.go, and returns each field's members in
  document order; the generated decoder decodes each into the field and
  `rt.JsonFoldErr.Note` keeps the error by the same rule. A folding
  type's XoJsonRead (the exact-key fast reader) returns false, so the
  tree decoder runs.
- Tests: golden json_fold (internal/llvm/testdata/golden: every backend
  and TestGoldenGoParity; wasm through `xo dev goldens --backend=wasm`),
  test/errorcheck/json_match, and internal/stdtest TestJsonFoldDiff, a
  differential against encoding/json (keys in random case, folding
  runes, escapes, duplicates, values that do not fit; seed 1 with 1,500
  cases and seed 7 with 4,000 cases equal on arm64, LLVM and the Go
  backend, 2026-11-07).

## Measured (2026-11-07)

`xo bench --ab`, linux/arm64 offload host 1, 9 rounds, 0 loaded, one
binary decoding a 10 field struct (`case: pascal`, Int fields with
defaults) 2,000,000 times, A the type without the option, B the same
type with `match: fold`, ns per decode (medians, B/A with its 95%
interval):

| input | without | with `match: fold` | B/A |
|---|---|---|---|
| (a) exact keys | 227 | 237 | 1.044 (1.039 to 1.07) |
| (b) every key lowercased | 258 | 350 | 1.357 (1.349 to 1.37) |
| (c) unknown keys | 254 | 361 | 1.421 (1.417 to 1.433) |

(a) is the cost of the option on input that uses the declared keys:
the exact probe is find_all's, the rest is the fold path's code size.
In (b) the type without the option matches nothing and decodes no field
(every field takes its default), while the folding type decodes all ten,
so the row overstates the matching cost; against (a)'s folding decode the
fold match costs 113 ns for 10 members. In (c) every member misses both
passes: about 11 ns per member. A type without the option runs the same
code as before (dec_fields_in is the old body; the option is chosen at
compile time). A first version that carried the error state as SSA
variables through every field cost (a) about a fifth on the Mac; the
fast path now jumps to one cold block on any error.

## Consequences

- Go config ports keep `derive Json` and Go's input compatibility, with
  one option per type; goversioninfo's hand decoder (about 200 lines)
  becomes a derive.
- A folding decoder is as lenient as Go: a key that folds to a field
  (`"MAJOR"`) is accepted.
- Known gap (agreed with xo-lead, 2026-11-07): a via type inside a
  nested struct that is not itself a via type counts as a type error at
  the outer level (DecodeErr carries no hard flag, and adding one would
  change a public std type), so an earlier type error in the outer
  object wins where Go would report the via error; golden json_fold's
  `Wrap` case pins today's behavior. If a real program needs Go's answer
  there, it comes back as its own ADR.
- Independent of 0543 (map key order on encode; confirmed with Xo Lang
  3, 2026-11-07): that changes encoding only.

## Alternatives

- **Fold always, as Go.** Matches Go with no option, but changes the
  meaning of every existing derive (a key that differs only by case
  starts to match) and gives every decoder a miss path. Rejected: the
  exact default is what Xo code expects.
- **A decode option (`json.decode_str[T](text, fold: true)`).** Per
  call, not per type, so nested types fold too as in Go; but every call
  site must remember it and every derived decoder must carry both paths.
  Rejected for the per-type option; could be added on top later.
- **Keep decoding Values by hand in each port.** No compiler or std
  work, but about 200 lines per config type and an unchecked copy of
  Go's rule in every port.
- **Exact match first, then fold, as a lookup over the whole object**
  (the port's first reading): picks an exact member even when a folded
  member comes later, which is not Go's rule 1 (`{"Major":1,"major":2}`
  gives 2 in Go). Rejected.
