# 0321: `??` with an early exit (`x ?? return v`), and the Result form

Status: Proposed (Xo Lang Lib 1, 2026-10-21), for the owner to decide.
The lead asked for this draft after the agents' poll of 2026-10-21.

## Context

Unwrapping an Option or a Result and leaving the function when it is
absent is the most common shape of Option code in std and compiler
(compiler/check/derive.xo, written 2026-10-21, has eleven of them). The
language offers:

- `opt?`, which returns `None` and works only in a function that returns
  an Option (core 2.5);
- `opt.or_err(e)?`, which returns `Err(e)` in a function that returns a
  Result;
- `opt ?? default`, where the right side may be `fault(...)` or any
  other `Never` call, since `Never` unifies with every type;
- a `match` or `if let ... else` for everything else: a different return
  value, `continue`, or `break`.

`return`, `break`, and `continue` are statements, so they cannot stand on
the right of `??`: `let s = x ?? continue` is `XO0160`. Code that wants
them writes four lines (`let s = match x { Some(v) => v  None =>
continue }`), which is the pattern repeated most in a loop body and in
`parse`-style functions that walk a list.

## Decision (proposed)

1. **Option form.** `return`, `break`, and `continue` are expressions of
   type `Never` where an expression is expected, as `fault(...)` is. So
   ```
   let s = x ?? return None
   let n = parse(t) ?? continue
   let row = rows.get(i) ?? break
   ```
   all check, with `??`'s left side an Option as before. A `return` with
   a value is checked against the function's result (or success type, as
   a statement `return` is). Nothing else changes: `let v = if c { 1 }
   else { return }` already works because the block's last statement ends
   the path (decision 0300).
2. **Result form.** The error has to be named, because it is usually
   wrapped. Spelling proposed: an arm after `??` on a Result,
   ```
   let v = fetch(id) ?? e => return Err(Wrapped(e))
   let w = parse(s) ?? e => continue
   ```
   (`e` is the error, `=>` as in a match arm, the right side any
   expression, a `Never` one to leave). The type of the whole is the
   success type. A Result on the left of `??` without an arm stays
   `XO0406` (today's rule), so `x ?? default` cannot silently drop an
   error.
3. **Vet.** `xo vet` flags the four line `match` that `??` now writes
   shorter (an advisory, never an error), with the fix.
4. **Formatting and grammar.** `Return`, `Break`, and `Continue` join the
   expression grammar at the precedence of `??`'s right side; `xo fmt`
   keeps them on one line.

## Consequences

- Option code in std/native, the compiler, and programs shrinks (the
  count of `match` blocks with one arm returning or continuing is the
  measure to take before implementing).
- A `return` inside an expression can hide an exit in the middle of a
  line. Mitigation: only `??` (and `if`/`match` arms, which already allow
  it) take one, and `xo vet` can list them.
- The Result form adds syntax; if the owner prefers not to, item 1 stands
  alone and Result code keeps `match` or `.or_err(...)?`.
- Compiler work: the parser (the three words as expressions), the
  checker (`Never` type, the path ends for 0300's move walk and the
  flow checks), ssa and gen lower them as the statement forms they
  already lower; the spec (core 2.5, 3.5, the grammar) and a formatter
  test.

## Alternatives

- **A `guard`/`else` statement**, `let Some(v) = x else { return None }`
  (Swift, Rust let-else). One more statement form that binds a name; it
  does not shorten the expression use (`foo(x ?? continue)`).
- **`?` generalized**: `x?` returning the function's own absent value.
  Already how `?` works for matching function types; it cannot choose the
  value or the keyword, which is the gap.
- **Only a function value**: `x.or_else(fn() ...)`. A closure cannot
  `return` from the function that wrote it.
- **Do nothing**: keep `match`. Correct and explicit, longer.
