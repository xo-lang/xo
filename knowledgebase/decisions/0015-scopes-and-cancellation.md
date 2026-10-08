# 0015: Scope exit rules, background tasks, observable cancellation

Status: Accepted (2026-10-07). Amended by 0032.

## Context
Agents could not tell what a timed out scope returns, whether early exit
cancels children, or how to run a daemon (an evictor) alongside a server.
Spec 7.2 said cancellation was unobservable, yet the retry task required a
`Cancelled` error. (Paper test 2026-10-07, [results](../../bench/results/paper-2026-10-07.md))

## Decision
- `s.spawn` (foreground) and `s.spawn_background` (cancelled when the body finishes).
- Normal body completion: cancel background, wait for all. Early exit: cancel all, wait.
- Unconsumed child errors propagate out of the scope as if by `?` (XO0801 if not convertible).
- `s.cancel()` cancels tasks, not the body.
- `scope ... timeout` removed; `task.with_timeout` returns `T ! Timed[E]`.
- Cancellation is sticky. Fallible blocking calls return their `Canceled` variant; infallible ones unwind.

## Consequences
- Removes the contextual word `timeout` (it collided with parameters named timeout).
