# 0538: Struct (and list) shaped destructuring assignment targets

Status: Accepted (owner, 2026-11-06, through the lead). Struct targets
built 2026-11-06 (Xo Lang 1): parser expr.xo parse_field_list, checker
stmt.xo struct_place, flow.xo target_parts, ssa and gen `destructure`;
golden destructure_struct. List targets: option (A), owner 2026-11-06,
built the same day: parser list literal `..` (ListLit.rest), checker
stmt.xo list_place; golden destructure_list, test/errorcheck/list_target.

## Context

Decision 0536 built destructuring assignment with a tuple of places as
its target (`(a, b) = f()`, `(xs[i], p.y) = ...`) and left list and
struct shaped targets (`[a, b] = xs`, `P{x, y} = p`) to their own
decision. The owner approved both on 2026-11-06, "with the same rules
as the pattern forms of `let`".

Those rules, as built (checked with xo 5b3617027):

- A struct pattern in `let` (`let P{x, y} = p`, `var P{x: px, ..} =
  p`) names fields by name, with `name: pattern` or the shorthand
  `name`; every field must be named unless the pattern ends in `..`
  (XO0302); an unknown field is the unresolved name error; a field
  that is not `pub` outside its module is XO0605; the pattern's type
  must be the value's (XO0302).
- A list pattern in `let` must be irrefutable (XO0303). A list's length
  is not known statically, so `let [a, b, c] = xs` and `let [f, ..] =
  xs` are both XO0303 ("`[]` is not covered"); only `[..]` is
  accepted. spec 3.5's example `var [first, ..] = xs` (decision 0384)
  did not compile for that reason.

## Decision

### Struct targets

An assignment's target may be a struct shaped list of places:

```
P{x, y} = p                   // existing x and y get p.x and p.y
P{x: st.lo, y: xs[i]} = p     // fields and elements as places
P{x, ..} = p                  // the other fields are not stored
(P{x, y}, n) = (p, 3)         // nested in a tuple, and tuples in it
P{x: _, y} = p                // `_` drops a field
```

1. **Grammar.** In a destructuring assignment's target, a struct
   literal's field may be written `name` alone (shorthand for `name:
   name`, the place being the existing binding `name`), and the field
   list may end in `..`. Each field's value is a place, `_`, or a
   nested tuple or struct target, as in 0536. The parser reads both
   forms in every struct literal; outside a target they are XO0160 (as
   the parser reported before: a struct literal writes `name: value`,
   and has no `..`), so struct literals in expressions do not change.
2. **The rules of `let`'s struct pattern.** The literal's type names a
   struct (XO0302 otherwise) and is the value's type (XO0401, the
   value's check, as for a tuple target); a field it does not have is the unresolved name error;
   every field is named unless the target ends in `..` (XO0302, with
   the same message as the pattern's); a field that is not `pub`
   outside its module is XO0605. A field named twice is the struct
   literal's duplicate field error.
3. **Places.** Each place follows 0536: assignable as a single
   assignment's target (XO0502 per place), the same place twice is
   XO0539, and a compound operator is XO0502. The field's type must be
   assignable to the place's type (XO0401).
4. **Order: 0536's.** The value is evaluated first and in full, then
   each named field is stored into its place in the order the target
   writes them, left to right; fields left to `..` or `_` are not
   read. This is `{ let t = value; place1 = t.f1; place2 = t.f2 }`.
5. **Lowering.** As 0536: the native backends and the Go backend
   destructure the value once and store each field (compiler/ssa and
   compiler/gen `destructure`); the flow analyses read the places
   through `assign_places` and `assign_pairs`.

### List targets: option (A) (owner, 2026-11-06)

Under `let`'s rules a list target can only be `[..]`, which stores
nothing: `[a, b] = xs` would always be XO0303. Three ways were put to
the owner on 2026-11-06, who chose (A): no run time fault and no special
case for list literals.

- (A) `let`'s rules exactly: the grammar takes a list target, and it is
  XO0303 unless irrefutable; list targets become useful only if
  `let`'s list patterns change. (Recommended by Xo Lang 1 and the lead.)
- (B) A list target faults on a length mismatch, as an index out of
  range does (Python's ValueError); for one rule, `let` and `var` list
  patterns would fault the same way, a larger change of its own.
- (C) As (A), but a list target is accepted when the value is a list
  literal of the same length (`[a, b] = [b, a]`), which the tuple form
  `(a, b) = (b, a)` already writes.

As built:

1. **Grammar.** A list literal may end in `..` (`[a, ..]`, `[..]`;
   `ListLit.rest`, AST codec v8); `..` elsewhere in it is XO0171. Outside
   a target a list literal's `..` is XO0160, so list literals in
   expressions do not change.
2. **The rule of `let`'s list patterns.** A list target, alone or nested
   in a tuple or struct target, is XO0303 ("a list target must match
   every list, but `[]` is not covered") unless it is `[..]`; `[]` and
   `[a, ..]` are XO0303 too. Its places are still checked as 0536's
   (XO0502, XO0539), so every error shows at once.
3. **`[..] = v`** evaluates `v` (in order with the other places of the
   target it is nested in) and stores nothing; the native and Go
   backends lower it as nothing to store.

Before this, a list target was XO0502 (not a place).

## Consequences

- `P{x, y} = p` replaces `x = p.x` then `y = p.y`, and reads as the
  `let P{x, y} = p` it parallels; agents porting Rust write it by
  habit.
- The parser accepts `name` and `..` in every struct literal and the
  checker rejects them outside a target, so an expression `P{x}` still
  fails with XO0160, now from the checker (its message names the target
  form). Tools that only parse (xo fmt) format the shorthand and `..`.
- The AST changes (`FieldInit.short`, `StructLit.rest`), so the AST
  codec's version changes and cached parses are read again once.
- spec 3.5's `var [first, ..] = xs` example is corrected to a pattern
  that compiles.
- A list target says why it fails (XO0303, pointing at `if let` and
  `match`) instead of "cannot assign to this expression" (XO0502); if
  `let`'s list patterns ever take a length check or a fault, list
  targets follow them.

## Alternatives

- **Struct targets in the parser only** (a target parsed as a pattern
  when `=` follows): patterns hold names, not places, so `P{x: st.lo}
  = p` would not be expressible, and the parser does not know a target
  until it reads `=`.
- **No shorthand in targets** (`P{x: x, y: y} = p` only): longer, and
  unlike `let P{x, y} = p`.
- **Positional struct targets** (`P(x, y) = p`): no other place in Xo
  reads struct fields by position.
