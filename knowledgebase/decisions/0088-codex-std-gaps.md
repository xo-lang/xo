# 0088: Typed cancellation from sleep, stoppable stdin, JSON tuples, Duration.parse

Status: Accepted (2026-10-09)

## Context

OpenAI Codex, writing the benchmark tasks from the spec
([codex-2026-10-09.md](../../bench/results/codex-2026-10-09.md)), hit four
std gaps:

1. `Clock.sleep` unwinds when the task is cancelled (core 7.2, decision
   0015), so a retry API whose error type has `Cancelled` cannot return it
   from its backoff sleep: "Returning FetchErr.Cancelled while Clock.sleep
   unwinds is a spec/API tension."
2. A `read_line` blocked on standard input could not be stopped: neither
   cancellation nor the stop request of two phase shutdown (decision 0042)
   ended it, because the Go backend's read of os.Stdin cannot be
   interrupted. The spec's own 7.5 example `select`ed on a `read_line`
   call, which is not a channel operation and never compiled.
3. Tuples had no JSON form (`List[(Str, Int)]` in a `derive Json` struct
   was XO0404).
4. There was no `Duration.parse`; Codex went through
   `json.decode_str[Duration](json.encode_str(text))`.

Decision 0015 already chose that a fallible blocking call reports
cancellation as its error type's `Canceled` variant and an infallible one
unwinds. That stays.

## Decision

1. **`Clock.sleep_or_cancel(d) -> () ! ClockErr`**, with the new prelude
   `enum ClockErr { Canceled } derive Display`, following the existing
   rule (every error type of a blocking operation has `Canceled`). When the
   task is cancelled before or during the sleep it returns
   `Err(ClockErr.Canceled)` instead of unwinding. `sleep` keeps unwinding.
   Cancellation stays sticky: the next cancellation point reports it again.
   Deferred code is shielded, so there it sleeps in full. `FakeClock` has
   it too (virtual time), and the Go, LLVM, and arm64 backends implement
   it (the Go runtime turns the sleep's unwind into the error; the C
   runtime's `xo_clock_sleep_or_cancel` waits without starting an unwind).
2. **Reading standard input is a cancellation point while it waits, and a
   stop request ends it.** `read_line`, `read_all`, and `lines` return
   `Err(IoErr.Canceled)` (the existing variant) when the task is cancelled
   or once a stop is requested (core 7.5 phase 1: "readers stop
   reading"), whether they are blocked or start later. On the Go backend
   one reader goroutine does the blocking reads; a waiting reader selects
   on its result, its task's cancellation, and the stop event, and input
   read after a reader gave up is kept for the next read. A line already
   in the buffer is read without the goroutine. The fake standard input of
   tests returns `Canceled` after a stop request too.
3. **A tuple is a JSON array** `[a, b, ...]`, one element per position, on
   every backend; decoding needs an array of exactly that length
   (`at $: expected array of 2 elements, got 3`). `()` stays `null`. A
   tuple implements `Json` when all its elements do; `OpenApi` and `Proto`
   still exclude tuples.
4. **`Duration.parse(s) -> Duration ! DurationErr`**, with the new prelude
   `struct DurationErr { input: Str }` whose `display` is the same text
   on every backend. The syntax is Go's `time.ParseDuration` after
   trimming white space, the form JSON decoding of `Duration` already
   accepted (both now share one parser per runtime).
5. **Habit diagnostic** for the call Codex wrote,
   `Duration.parse(s).or_err("...")`: `or_err` on a `Result` gets a note
   (`or_err` converts an optional) and a machine fix to
   `.map_err(fn(_) { ... })`, instead of a "did you mean `is_err`" fix
   that made the code worse. No meaning of `XO0601` changes.

## Consequences

- A retry loop can map cancellation to its own error with one `match`;
  `FetchErr.Cancelled` is reachable.
- A reader of standard input stops at the first signal and lets workers
  drain; core 7.5's example now compiles. A program that kept reading
  standard input during the grace period now gets `Canceled` instead (the
  spec already said readers stop reading).
- The Go backend starts one goroutine on the first blocking stdin read; a
  read that gave up leaves it blocked until input or end of input, which
  is harmless (the process exits normally).
- Codex's reconstructed failing first run of task 10 (bench/habits) is
  green after `xo fix`; 6 of 7 reconstructed runs are now (5 before).
- Native backends: `sleep_or_cancel`, `Duration.parse`, and tuple JSON are
  implemented; standard input is still unsupported there (XO1201,
  planned), as before.

## Alternatives

- **Make `sleep` return `() ! ClockErr`.** Breaks every existing call and
  forces `?` onto sleeps that never care. Rejected.
- **A generic `struct Canceled {}` error.** Breaks the rule that each std
  error type has its own `Canceled` variant and adds a second spelling of
  the same thing. Rejected.
- **A selectable stdin channel (`stdio.lines_chan()`).** Needs a
  background reader that may consume input nobody receives, and a second
  API for the same reads. The cancellation point gives the same stop with
  the existing call. Rejected for now.
- **A new `IoErr.Stopped` variant for the stop request.** Would break
  exhaustive matches on `IoErr`, and the program treats both the same way.
  Rejected.
- **`Duration.parse(s) -> Duration?`,** like `Int.parse` and
  `Time.parse_rfc3339` (and what Codex assumed with `.or_err`). Rejected
  because the error can say what a duration looks like, which an agent
  needs more than an integer parse does; the habit fix covers the guess.
- **Tuples as objects `{"0": a, "1": b}`.** Not what Python, TypeScript,
  or serde produce for tuples; arrays are. Rejected.
