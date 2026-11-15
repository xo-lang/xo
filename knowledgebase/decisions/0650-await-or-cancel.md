# 0650: `t.await_or_cancel()` reports a cancelled task as a value

Status: Accepted (owner, 2026-11-15; drafted by aws-43 for the lead, a
follow-up of the owner's language survey). Not built yet.

## Context

`t.await` gives `T ! E` (core 7.1). When the awaited task was cancelled
before it finished, an awaiter that is not itself cancelled faults with
"awaited a task that was canceled before it finished" (std/rt/scope.go
Await, std/native/llrt/tasks.xo, compiler/wasm/rt3.xo); a cancelled
awaiter unwinds (core 7.2). Neither can be handled. The case is
ordinary in cancellation tests: this week's std/sql KILL QUERY and
CancelRequest tests (std/native/llrt/mysql_test.xo, pg_test.xo) cancel a
query task and want to see what the query returned. They could not
await it, so the task wrote into a `Chan[Str]` before its cancellation
point and the test read the channel; a `Mutex.with` storing the result
after the point never ran, as cancellation is sticky.

Two precedents report cancellation as a value without changing the
blocking form: `clock.sleep_or_cancel(d) -> () ! ClockErr` beside
`clock.sleep` (decision 0088), and `task.with_timeout`'s `T ! Timed[E]`.

## Decision

- **`fn (t: Task[T, E]) await_or_cancel() -> T ! task.Awaited[E]`**
  (prelude), with `pub enum Awaited[E] { Canceled  Failed(from E) }
  derive Display` in std/task beside Timed. It waits as `t.await` does
  and gives `Ok(v)`, `Err(Failed(e))` for the task's own error, or
  `Err(Canceled)` when the task was cancelled before it finished or when
  the awaiter is cancelled while it waits. As with `sleep_or_cancel`,
  cancellation stays sticky: the awaiter's next cancellation point
  reports it again. A task that faulted still faults the awaiter (a
  fault is not an error).
- `from E` gives `?` the conversion: `t.await_or_cancel()?` in a
  function whose error has a `From[Awaited[E]]` works as for Timed.
- **Consuming the error.** A handle bound with `let` and given to
  `await_or_cancel` has its error consumed, as with `await` (core 7.1's
  child failure rule).
- **`t.await` is unchanged**: it still faults on a cancelled task, so
  no existing program changes behavior.
- Inside deferred (shielded) code it waits in full, as `sleep_or_cancel`.

## Consequences

- Additive: no existing code changes. The three runtimes' await paths
  take a flag: where they fault or unwind today they return the
  Canceled value (rt `Await`, llrt tasks.xo's await, wasm rt3's).
- std/task gains `Awaited[E]`; spec core 7.1 and 7.2 add a line each;
  std.md 7 lists the enum.
- The tests of this week become `match q.await_or_cancel() { Err(Canceled)
  => ..., Ok(r) => ... }` instead of a side channel.
- Tests: a golden on every backend for each outcome (Ok, Failed,
  Canceled before finishing, the awaiter cancelled while waiting,
  sticky afterwards, a fault still faulting, inside defer).

## Alternatives

- **Change `t.await` to `T ! Canceled[E]`.** Every await site's error
  type changes (each `?` needs a conversion), a large breaking change
  for a case most code never meets; rejected.
- **`t.await` gives `Err` when E has a `Canceled` variant.** Behavior
  that depends on a variant's name is magic and silent when misspelled;
  rejected.
- **A non-blocking `t.state() -> TaskState`** (Running, Done, Canceled,
  Failed). It answers "was it cancelled" but not "what did it return",
  and invites polling; it can come later on its own.
