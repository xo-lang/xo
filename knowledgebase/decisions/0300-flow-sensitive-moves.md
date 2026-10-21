# 0300: Flow-sensitive moves of move-only builders

Status: Proposed (xo-fa, 2026-10-21, drafted at the lead's request for
the owner; no code until the owner approves).

## Context

`Buf[T]`, `StrBuilder` and `BytesBuilder` are move only (core 5.3):
assigning, passing, returning, freezing or capturing one moves it, and a
later use is XO0510 "use of moved value". The check is
compiler/check/flow.xo's move walk (check_moves, mv_block, mv_stmt,
mv_loop, mv_expr). Its state is the set of locals that may have been
moved, with where each was moved:

- the branches of `if`, `match` and `select` each start from the state
  before them, and the state after them is the union of the branches';
- a loop body is walked twice, so a move in one iteration is seen by the
  next, and the state after the loop is the union with the state before
  it (the loop may not run);
- `let`, `var` and a plain assignment `b = ...` clear the binding's
  moved state.

Nothing ends a path. A `return`, `break`, `continue`, the failing side
of `?`, or a call whose result is `Never` (`fault`, `os.proc.exit`)
leaves its moves in the state that flows to the code after it, though
that code cannot run after them. So the natural shape of a decoder is
rejected:

```
fn decode(s: Bytes) -> Bytes? {
  var out = BytesBuilder.new()
  for c in s {
    if c == 61 {
      return Some(out.freeze())   // moves out, then the path ends
    }
    out.write_byte(c)             // XO0510 today: out "was moved" above
  }
  Some(out.freeze())              // XO0510 today
}
```

The same holds for `if bad { return Err(...) }` after a freeze in a
branch, for an early `return (out.freeze(), err)` inside a `while`, and
for a `match` arm that freezes and returns. The workarounds are a flag
or an error variable, `break`, and one freeze after the loop: std's
quoted-printable decoder (std/native/multipart.xo qp_decode) keeps its
error in `err` and breaks out so its single `(out.freeze(), err)` comes
after the loop. Two gotchas record the shape:
gotchas/freeze-then-return-in-loop.md and
gotchas/move-check-loop-local-builder.md.

The agent poll of 2026-10-20 (knowledgebase/drafts/agent-poll-2026-10-20.md)
names this the largest time cost for the std work: multipart's
read_part, qp_decode, the uuid parser and the base64 decoder were each
written, rejected with XO0510 and restructured.

## Decision (proposed)

The move walk tracks whether its path can still continue. The rule for
users, added to core 5.3:

> A move on a path that ends (a `return`, a `break` or `continue`, the
> error side of `?`, a call of a function whose result is `Never`) does
> not reach the code after that point. A use is XO0510 only when some
> path that can reach it moved the value and did not assign it again.

What the walk does:

1. The state gets a "dead" mark: after `return`, `break`, `continue`,
   or an expression statement whose type is `Never`, the path is dead.
   A dead path reports nothing, and a join ignores it: the state after
   an `if` whose `then` block returns is the `else` block's state (or
   the state before the `if` when there is no `else`).
2. `break` hands its state to the loop's exit and `continue` to the
   loop's next iteration. The state after a loop is the union of the
   state before it (zero iterations), the state at the end of the body,
   and every `break`'s. The next iteration starts from the union of the
   end of the body and every `continue`'s. The body is walked until the
   state stops growing; the moved set only grows and each walk is the
   same function of its input, so this stops after at most one walk more
   than the number of move-only locals. Today's two walks are the case
   of a loop with no early exit.
3. `x?` moves what `x` moves on both sides. Its error side ends the
   path, so it adds nothing a later use could see. `return` inside a
   closure body ends the closure's path only. A closure body is walked
   on its own, as today.
4. A labeled `break` or `continue` (`outer: for ... { break outer }`)
   hands its state to the loop it names.
5. Nothing else changes: moves are still "may" moves (a move on any
   reaching path is an error), assignment still re-initializes, there
   is no borrow checker, and XO0510's code and meaning stay. Its note
   still points at a move on a path that reaches the use.

The checker accepts strictly more programs. Code generation is
unchanged: a path that ends does not reach the use at run time, so the
builder's reference counts and frees are the ones the SSA builder
already emits for these paths.

## Consequences

- The decoder shapes above compile as written. std's workarounds can be
  simplified but need not be: nothing rejected today becomes accepted
  with another meaning.
- The walk costs a little more on loops with early exits (more than two
  walks). Move-only locals are few, and the walk runs only for bodies
  that name one (names_move_only), so the check time of xo and std is
  not expected to move. This is measured on `xo check` of xo before and
  after.
- Spec: core 5.3 gains the paragraph above and the example. Appendix B
  is not involved.
- Tests: compiler/check rules tests for each item: return in a loop, in
  an `if` without `else`, in a `match` arm, `?` after a freeze, a
  `Never` call, `break` then a use after the loop (still XO0510),
  `continue` after a freeze (still XO0510 at the loop head when the
  binding is not assigned again), labeled loops, and a closure's return.
  Plus a golden that runs a decoder of the natural shape on every
  backend.
- The two gotchas are marked fixed by this decision when it lands.

## Evidence (2026-10-21, for the owner)

- What it buys is the time to write the code, not run time: the rule
  only accepts more programs, and code generation is unchanged (see
  Decision), so no profile can show a gain. `xo vet`'s copy-cost notes
  are about something else (XO0534, a task capturing a value of 32 or
  more heap fields, decision 0294; there is no XO0536); over compiler/
  and std they report none (compiler: 47 XO0208, 8 XO0305, 2 XO0532,
  1 XO0207; std: 24 XO0703, 6 XO0208, 1 XO0530, 1 XO0531, 1 XO0207).
- Of the 292 functions in std and compiler/ (tests excluded) with a
  move-only builder local, a scan for the workaround shape (one freeze,
  a loop left by `break`, an error or flag variable set before it)
  finds std/native/multipart.xo read_part and qp_decode, and
  std/native/mail.xo consume_quoted_string; it also matches
  compiler/arm64/partcache.xo part_keys, which is not that shape. The
  scan cannot see code that was restructured in other ways (a helper
  returning early, a builder made per iteration); the agent poll of
  2026-10-20 names those (the uuid parser, the base64 decoder).
- None of these is in the hot paths of the profiles we have (plain HTTP,
  TLS, xo's own build): multipart and mail parsing do not run there.

## Alternatives

- Keep the rule and document the workaround: today's state, and the
  largest time cost the std agents report.
- A general borrow checker or ownership types: far larger than the
  problem; core 5.3 rules it out ("no general borrow checker").
- Liveness-based moves (a moved value counts only while it is used
  later, as non-lexical lifetimes): handles the same shapes, but its
  errors are harder to explain and it needs a liveness pass the checker
  does not have. Path ends cover every case in the poll.
- Implicit re-initialization after `freeze` (a frozen builder is a new,
  empty one): changes what the program means and hides real double uses.
