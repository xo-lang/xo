# 0258: discarding the result of a call that only computes a value

Status: Accepted (the owner, 2026-10-17, through the lead: an error,
XO0209). Proposed by Xo Lang 2 the same day; implemented in
compiler/check (stmt.xo expr_stmt and report_discards).

## Context

`xs.reverse()` returns a reversed copy (spec/std.md). Written as a
statement, it compiles and does nothing, because "a non `Result` value
may be discarded as an expression statement" (core 2.6). In
compiler/arm64 fieldlicm.xo this left a list backward, and the symptom
was a module verification failure far from the cause
(gotchas/list-reverse-returns-copy.md).

The trap is general. Value semantics make every collection and string
method that "changes" a value return a new one: List sort, sort_by,
reverse, map, filter; Str trim, replace, to_upper; Map and Set filter.
Agents trained on languages where `list.reverse()` and `list.sort()`
mutate in place (Python, Java, C#) write exactly this statement. Xo
already refuses the same mistake for errors: an unused `Result` is error
XO0201.

## Decision

1. New code XO0209, an **error** from the checker. It reports an
   expression statement whose value comes from a call that only computes
   that value. The message: "the `List[Int]` that `xs.reverse(...)`
   returns is discarded, and the call does nothing else".
2. Which calls count. Purity is derived from what the checker already
   knows, with no marker in std (the owner's preference: use the effect
   signatures the prelude already carries). A call counts when all of
   these hold:
   - it performs no effect: the call's effects, collected in a set of
     their own while the statement is checked, are empty once effects
     are solved (solve_effects; inferred effects of non pub functions
     included). Prelude methods declare theirs (`uses fx` on map, filter,
     sort_by), so `xs.map(f)` with an effectful `f` is not reported;
   - it mutates nothing: no `var` receiver or `var` argument
     (require_mutable did not run for it), so `xs.push(x)` is not
     reported;
   - it reaches no state through its operands (reaches_state). None of
     the receiver, the arguments, or the called function value holds, at
     any depth, a handle (Atomic, Chan, Mutex, Scope, Task, Listener,
     Conn, File, FileLock, Child, or an opaque C or native type), an
     interface value, a function value, or a type not known at that
     point (a type parameter). Handles change shared state without an
     effect label: `counter.add(1)`, `done.recv()` and `s.spawn(...)`
     return values and are not reported;
   - its value is not unit, Never, or a `Result` (XO0201 and XO0206
     cover a Result), and it is not `.ignore_err("reason")`, which is how
     a Result is discarded.
3. Native std modules (std/native, decision 0158's llrt) are exempt.
   Their memory and system intrinsics (`le32`, `close`, `unlink`) write
   memory or make system calls without effect labels.
4. Fixes:
   - When the receiver is a `var` binding of the result's type, the
     first fix assigns the result back: `xs = xs.reverse()`. `xo fix`
     applies a diagnostic's first fix.
   - Otherwise, and as the second fix, `let _ = ...` discards the value
     on purpose. `let _ =` is accepted for these calls, since there is no
     failure to justify.
   - A `defer` statement's call is not reported: it runs for what it
     does at exit, or to fault there (fault tests defer a faulting
     helper; progen defers `s.trim()`).

## Consequences

- Corpus, checked before the change, over every `.xo` file in the
  repository except runs/ and output/, std/native checked with
  `--native`: no hits once `defer` statements are exempt.
  - Without the exemption there were seven hits, all in one golden
    (internal/llvm/testdata/golden/fault_defer_chain): `defer boom(...)`
    calls made for their fault. Rewriting them as
    `defer { let _ = boom(...) }` exposed a native backend bug (a fault
    in a deferred block skips the remaining defers,
    gotchas/native-deferred-block-fault-skips-defers.md). That is one
    reason for the exemption.
  - The first version of the rule, without the state test, reported
    about 360 sites, all false. They were Atomic.add, channel receives,
    spawns, a test helper writing through a store handle, and
    ignore_err. That is why point 2's state test exists.
  - No hits in non native std, compiler/, examples, bench/ or tests.
    bench/paper is unchanged.
- spec/core.md 2.6 states the exception to "a non `Result` value may be
  discarded"; section 10 lists XO0209; `xo explain XO0209` describes it.
- A generic function that discards `xs.reverse()` on a `List[T]` is not
  reported, because T might hold a handle. That is a false negative, not
  a false positive.

## Alternatives

- A warning (vet only). Rejected by the owner: the statement is never
  useful, and warnings are skipped.
- Make reverse and sort mutate a `var` receiver in place. That changes
  the std surface and value semantics for one family of methods, and
  leaves map, trim and the rest with the same trap.
- A purity marker on std declarations (`#[pure]` or a doc tag). Not
  needed: effect signatures, `var` receivers and the state test decide
  it, and a hand kept list would drift from std.
