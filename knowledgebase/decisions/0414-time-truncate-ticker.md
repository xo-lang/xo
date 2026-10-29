# 0414: std/time: truncate and round, Weekday numbers, a Ticker

Status: Accepted (lead, 2026-10-29: 0.35 std survey rank 17; Xo Lang 2)

## Context

The std survey for 0.35 ranked three gaps in std/time against Go's
package 17th: rounding an instant to a duration (Go's `Time.Truncate`
and `Round`, used for bucketing metrics, log rotation, and cache keys),
turning a `Weekday` into a number and back (Go's `Weekday` is an int;
Xo's is an enum with no conversion), and a periodic tick (Go's
`time.Ticker`). `Time` is a prelude type whose methods the compiler
lowers itself (compiler/ssa call.xo), so a new method on it is compiler
work and a new seed's worth of intrinsics.

## Decision

- `time.truncate(t, d)` and `time.round(t, d)` are functions of
  std/time, not methods of the prelude's `Time`: no compiler change, and
  `use std/time` already names the module where a reader looks for
  calendar work. They port Go's arithmetic: multiples of `d` counted from
  the zero time (January 1 of year 1, UTC, 62,135,596,800 seconds before
  the Unix epoch), so the answer equals Go's for every `d`, including
  ones that do not divide a day (7,919 ns, 3,000,000,005 ns, 2**50 ns);
  `d <= 0` returns `t`; `round` rounds halves up. Native computes the
  remainder without overflow (a modular add and multiply over the zero
  time's offset), the Go backend calls Go's methods.
- `Weekday.number()` is the ISO 8601 number (Monday 1 to Sunday 7), and
  `time.weekday_from(n) -> Weekday?` its inverse, `None` outside 1 to 7.
  Go's Sunday 0 numbering is not offered: ISO is what calendars, cron
  fields (1 to 7 accepted), and other languages' week APIs share, and one
  numbering avoids an off by one between two.
- `time.Ticker` is a struct, not a channel and a background task:
  `time.ticker(clock, period)` makes one whose ticks are at its start
  plus multiples of `period`; `t.wait()` sleeps until the next tick and
  returns its scheduled time; `t.reset(period)` returns a ticker whose
  first tick is one new period from now. There is no `stop`: a ticker
  nobody waits on does nothing. It keeps the next tick in an
  `Atomic[Int]` so copies share one schedule, and `wait` claims its tick
  with compare_and_swap before it sleeps, so each tick reaches one waiter
  as each value of Go's channel reaches one receiver. A caller that falls
  behind gets the pending tick at once and the missed ones after it are
  dropped with the phase kept, as Go's channel of one does. A period of 0
  or less faults with Go's panic text. It runs on any `Clock`, so under
  a `FakeClock` the ticks are virtual and tests are deterministic.

## Consequences

- Spec std 13.6; std/changes/time.md; the golden time_truncate_ticker
  (arm64, LLVM, Go backend) and std/native/time_test.xo.
- A select over a ticker and another channel needs a task that waits on
  the ticker and sends; a later decision can add a `Chan[Time]` view if
  programs want it.
- `Time.truncate` as a method stays possible later (the compiler would
  lower it to the same function).

## Alternatives

- Methods on `Time`: rejected for now (compiler lowering and a seed for a
  calendar helper).
- A Go shaped `Ticker` with a channel `c` fed by a background task:
  rejected; it needs a task group to own the task and a `stop` to end it,
  and a missed `stop` leaks a task; the struct form has neither problem.
- Go's Sunday 0 weekday numbers, or both numberings: rejected above.
