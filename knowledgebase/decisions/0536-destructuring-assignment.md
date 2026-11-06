# 0536: Destructuring assignment to existing places (`(a, b) = f()`)

Status: Accepted (owner, 2026-11-06, through the lead: "go with your
recommendations"), with value first evaluation order (item 5, Python's
order, which replaced the proposal's Go order). Built 2026-11-06 (Xo
Lang 1): the checker types each place and checks the value against the
tuple of their types (compiler/check/stmt.xo tuple_assign), the flow,
escape, undone and realtime analyses read the places through
assign_places and assign_pairs (flow.xo), and the native backends and
the Go backend evaluate the value once, then store each element
(compiler/ssa/stmt.xo and compiler/gen/fn.xo destructure); golden
destructure_assign.

## Context

Xo destructures only into new names: `let (q, r) = divmod(a, b)` and,
since decision 0384, `var (lo, hi) = (0, 10)`. Assigning a tuple to
names that already exist is not possible. Today `(a, b) = f()` parses
as an assignment whose target is a tuple expression, and the checker
rejects it with XO0502, "cannot assign to this expression; only
bindings, fields, and elements of `var` bindings can be assigned"
(compiler/check/stmt.xo, place_type's fallback).

What code does instead, measured 2026-11-06 over the non-test `.xo`
files of compiler/ and std/ (731 files, 2,705 `let (...) =`
statements):

- 395 destructures are followed, within the next few lines, by an
  assignment of one of the new names to a place that already exists:
  232 to plain variables, 163 to a field or an element
  (`streams[PS_TPI] = tpi`, compiler/macho/pdb.xo:676;
  `ob.syms = kept_syms`, compiler/macho/elfhalf.xo:175). 147 of them
  assign every name, so the whole statement is a destructuring
  assignment written as two or more.
- 44 swaps through a temporary: `let t = ra; ra = rb; rb = t`
  (compiler/llvm/noalias.xo:125), `let t = loops[i]; loops[i] =
  loops[j]; loops[j] = t` (compiler/arm64/fieldlicm.xo:44).

Loops that carry two values (Fibonacci style `(a, b) = (b, a + b)`, a
parser's `(tok, pos) = next(src, pos)`) need a temporary per name, and
the temporary's name is a common place for a mistake (assigning the old
value to the second name).

Other languages:

- Go: `a, b = f()`, `a, b = b, a`, `xs[i], xs[j] = xs[j], xs[i]`. The
  index and pointer operands on the left and the expressions on the
  right are evaluated first, then the assignments happen left to right.
- Python: `a, b = b, a`, any target (names, attributes, subscripts).
- Rust (1.59): `(a, b) = (b, a)`, `[x, y] = arr`, struct patterns; `_`
  discards.
- Swift: `(a, b) = (b, a)`. Kotlin: no destructuring assignment.
- JavaScript: `[a, b] = [b, a]`.

## Decision (proposed)

An assignment statement may have a tuple of places as its target:

```
(a, b) = (b, a)                     // a swap
(q, r) = divmod(n, d)
(xs[i], xs[j]) = (xs[j], xs[i])
(st.lo, st.hi, _) = bounds(st)      // `_` discards
((x, y), n) = (pt, count)           // nested
```

1. **Grammar.** `TARGET = EXPR` where TARGET is a place (as today) or a
   parenthesized, comma separated list of two or more TARGETs or `_`.
   The parser already reads it (an assignment whose target is a tuple
   expression); `_` is accepted there and nowhere else in an
   expression. Only `=`: a compound operator with a tuple target
   (`(a, b) += (1, 2)`) is an error (XO0502 with its own message).
   List and struct shaped targets (`[a, b] = xs`, `P{x, y} = p`) are
   not part of this decision: a list's length is not known statically,
   and struct targets have had no request.
2. **Types.** The value's type must be a tuple of the same arity (nested
   to the same shape). When the value is a tuple literal, each element
   is checked against its target's type, so `(a, b) = (b, a)` and
   `(n, s) = (0, "")` infer as two single assignments would.
3. **Mutability: the XO0502 change.** Every place in the target must be
   assignable as a single assignment's target would be: a `var`
   binding, a `var` parameter, or a field or element of one, and not a
   captured variable inside a closure (3.3). Each place that is not
   reports XO0502 at that place, not once for the whole tuple, with the
   existing fix (declare the binding `var`). XO0502's message for an
   expression that is not a place gains "or a tuple of those". A name
   that does not exist is XO0601, and when none of the names exist the
   fix offers `let (...) =` (the statement probably meant to declare).
4. **The same place twice** is an error: `(a, a) = (1, 2)` and
   `(p.x, p.x) = ...` are a new code, XO0539 if still free when built
   ("assigned twice in one destructuring assignment"). Places that only
   may overlap at run time (`(xs[i], xs[j])`) are allowed, unlike two
   `var` arguments (decision 0111): the stores happen in order, so the
   result is defined (the later store wins, as in Go), and a swap of
   two elements is the main use.
5. **Order: the value first (accepted; Python's order).** The value is
   evaluated first, then each target in turn, left to right: its
   indexes, then its store, as a single assignment of that element
   would (decision 0140: a store reads its place's collection when it
   stores). This is exactly `{ let (t0, t1) = value; a = t0; b = t1 }`,
   which is how it is built (item 6), so every check that reads single
   assignments (definite assignment, XO0520's lost updates, escape)
   applies unchanged. It differs from Go's order (every target's
   indexes before the value) only when the value changes a variable a
   target index reads, through a `var` argument. A fault in a store
   (an index out of range, or a missing Map key under a field or
   element target, which XO0410 reports per target as for a single
   assignment) leaves the earlier stores done, as in Go and Python.
6. **Lowering.** The checker rewrites it, after checking the target's
   types, to the value bound to fresh temporaries, then one single
   assignment per target in order (each temporary annotated with its
   target's type, so item 2's per element check holds): no tuple is
   built when the value is a tuple literal (the elements are SSA
   values), so a swap costs two moves. Every backend
   gets it through the rewrite.
7. **`var` and `let` declarations do not change.** A destructuring
   assignment never declares: mixing new and existing names (Go's `:=`
   with an existing name) is not allowed; declare the new names first,
   or bind all with `let`/`var`.

Tests when built: a golden on every backend for the swap of two locals,
two list elements (equal and different indexes), a nested target,
`_`, a Map element, a field of a `var` struct, a value that changes the
target's binding (0140's case), and a fault in the second store;
checker tests for XO0502 per place, the compound operator, XO0539 and
XO0601's fix; spec 3.1 and 5.2 and the code table.

## Consequences

- The 44 swaps and the 147 statements that assign every destructured
  name become one statement each; the rest can drop their temporaries
  where they want to. No existing program changes meaning: every
  accepted target today is still accepted with the same order.
- One more form for agents to know; it reads as Go's and Rust's do, so
  agents porting Go write it by habit (`a, b = b, a` becomes
  `(a, b) = (b, a)`; the parentheses are required, as in Rust, so a
  comma never starts an assignment).
- XO0502's message and fix change wording; its meaning does not.

## Alternatives

- **Keep it out** (today): `let (a0, b0) = f()` then `a = a0`, and a
  temporary for each swap. No language change; the measured 395 and 44
  sites stay as they are.
- **Go's unparenthesized form** (`a, b = b, a`): a comma at statement
  start would be new in the grammar and close to a tuple expression
  statement; the parenthesized form needs no parser change.
- **A `swap` function or method** (`swap(var a, var b)`,
  `xs.swap(i, j)`): covers swaps only, not `(tok, pos) = next(...)`;
  two `var` arguments of overlapping places are XO0503 (0111), so a
  general `swap(xs[i], xs[j])` would need an exception.
- **Allow list and struct targets now** (Rust): more surface without a
  measured need; can follow as its own decision.
