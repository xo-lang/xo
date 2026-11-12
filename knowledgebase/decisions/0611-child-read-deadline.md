# 0611: Child read deadline (Go's SetReadDeadline on a child's pipes) and ProcErr.Timeout

Status: Accepted (Xo-lead, 2026-11-11, relaying the scarlet session's field report and naming Go's shape; follows decision 0430, whose shape the lead and Xo Lang 3 agreed for connections). Xo Lang 3.

## Context

The scarlet session (real-time audio on Xo) reads a speech engine's output with `Child.read_line`. A barge in (the user speaks over the reply) has to act within milliseconds, but `read_line` waits for the next line with no bound. The workaround was a pump task per child forwarding lines into a channel and a `select` with `after 5ms`: a task, a channel and a copy per line for what Go programs do with `os.File.SetReadDeadline` on the pipe.

Connections already have Go's deadlines (decision 0430: `Conn.set_read_deadline(clock, t)`, `NetErr.Timeout`), on the same poller natively.

## Decision

1. `fn (c: Child) set_read_deadline(clock: Clock, t: Time?) uses clock`: the deadline of reads of the child's output and error pipes (`read`, `read_err`, `read_line`), Go's `SetReadDeadline` on both pipes; `None` removes it.
2. A read that would wait past it fails with the new variant `ProcErr.Timeout` (display `i/o timeout`, Go's `os.ErrDeadlineExceeded` text), and so does every later read until the deadline moves; a deadline already passed fails at once, even with output in the pipe (as 0430 for connections). A deadline set while a read waits applies to it.
3. Nothing is lost: bytes `read_line` read ahead, including the start of a line read before a timeout, are returned by the next read.
4. The child keeps running; the caller decides (keep reading, `kill`).
5. A FakeProc child's reads never wait and ignore the deadline.

## Consequences

- `ProcErr` gains a variant: an exhaustive `match` on it needs a `Timeout` arm (CHANGELOG **Breaking**). One site in compiler/ (cli/servers/dump.xo) gains it.
- Native (llrt/proc.xo): a word `CH_RDL` in the child record on the poller's clock; `ch_read` checks it first and `step_wait` waits on the poll descriptor until the earlier of it and the 1 ms input feeding step; `set_read_deadline` kicks waiting readers (`poll.pd_kick`), as netconn does. Windows' polled pipes check it after each step.
- Go backend (std/rt/proccmd.go): `SetReadDeadline` on the pipe files, restored after a cancellation (`interruptibleTo`); `os.ErrDeadlineExceeded` is `Timeout`; a timed out `read_line` keeps its partial line.
- Golden proc_read_deadline (arm64, llvm, Go parity): timeout, repeat, clear, past deadline, error stream, half line kept, barge in.
- spec/std.md 3.8 and core 7.5 (a blocking `use c` call is not a cancellation point; poll `task.stop_requested()` per chunk).

## Alternatives

- A select-able read (`select { line = c.lines() => ..., after 5ms => ... }`): fits Xo's select, but needs a channel or a read future per child, with a task feeding it (today's workaround made built in) and the ownership of a half read line between arms. The deadline is Go's answer, costs one word per child, and composes with select users who still want a pump.
- `read_line_within(d)`: one call shape per read method (three), and a deadline per call loses Go's "set once, every read" model that 0430 already chose for connections.
- `Io("i/o timeout")` instead of a variant: no breaking change, but callers would match on text; NetErr has `Timeout` for the same case.
