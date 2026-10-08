# 0017: Implicit test os; std/test renamed std/testing

Status: Accepted (2026-10-07)

## Context
All 5 agents had to guess how tests get fake capabilities and how to drive
them (advance a clock, register a network handler, inject failures).
`use std/test` bound the keyword `test`. (Paper test 2026-10-07, [results](../../bench/results/paper-2026-10-07.md))

## Decision
- Test blocks predeclare `os: TestOs`, `expect`, `testing`. Test bodies are `() ! Error`.
- Fakes: FakeNet (handlers, failures, request log, in memory listen), FakeClock
  (virtual, auto advancing, `advance`, `sleeps`), MemFs (`fail_next`), FakeEnv, FakeStdio, FakeProc.
- `expect(x) <cmp> y` with any comparison; `expect(cond)`.
- Module renamed `std/testing`.
