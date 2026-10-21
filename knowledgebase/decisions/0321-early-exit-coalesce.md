# 0321: `??` with an early exit (`x ?? return v`), and the Result form

Status: Accepted for part 1 (owner, 2026-10-21): `return`, `break`, and
`continue` as `Never` expressions on the right of `??` (the Option form).
Part 2, the Result form (`?? e => ...`), is deferred: Result code keeps
`match` and `.or_err(...)?`. Drafted by Xo Lang Lib 1 after the agents'
poll of 2026-10-21.

Measure (2026-10-21, origin/main): 251 one-arm `match` blocks of the shape
`let x = match opt { Some(v) => v  None => return|continue|break ... }`
in std/native and compiler/ (218 `return`, 28 `continue`, 5 `break`; 129
in std/native, 49 in compiler/check, 49 in compiler/cli, 7 in
compiler/arm64, 6 in compiler/gen, 6 in compiler/ssa), each four lines
that `x ?? return v` writes in one.

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

## Decision (part 1 accepted, part 2 deferred)

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

## Implementation notes (2026-10-21)

- The parser turns `x ?? return v` into `x ?? { return v }`: a Block
  expression of the one statement, with `Block.exit` set so `xo fmt`
  prints it back without braces. Nothing below the parser changed: the
  checker's block typing (a path that ends is `Never`, decision 0300),
  ssa, gen, and wasm see an ordinary block on the right of `??`.
- `return`'s value stops at `)` and `]` too, so `f(x ?? return)` parses.
- `xo vet` XO0906 (a note) finds the four line `match` and offers the
  rewrite as a fix.
- Tests: errorcheck coalesce_exit (non-Option left side `XO0406`, a
  wrong return type `XO0401`, `continue` outside a loop `XO0607`, and a
  move on the exit path that the continuing path still uses, accepted);
  golden coalesce_exit on go, arm64, and LLVM.
- The compiler cannot use the syntax until a seed has it (gotcha
  new-syntax-needs-a-newer-seed); rewriting the 251 blocks follows the
  next seed cut.
- Part 2, the Result form, stays deferred.

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
