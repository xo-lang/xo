# A blocked read of standard input ignores cancellation and signals

Status: Observed (2026-10-09, Codex's log pipeline task)

Symptom: a program reading lines with `os.stdio.read_line()` did not stop
at SIGTERM, and cancelling the reader's scope did not end the read: the
task stayed blocked until a line or end of input arrived. Core 7.5 said
"readers stop reading", and its example `select`ed on a `read_line` call,
which is not a channel operation and never compiled.

Cause: the Go backend called `bufio.Reader.ReadString` on os.Stdin
directly. A read of os.Stdin cannot be interrupted from another goroutine
(closing os.Stdin does not reliably wake a pending read on a terminal or
pipe), so the cancellation check before the read was the only one.

Fix (decision 0088): std/rt/stdin.go hands blocking reads to one reader
goroutine and waits on its result, the task's cancellation, and the stop
event, returning `IoErr.Canceled`; what the goroutine reads after its
reader gave up is kept for the next read, so input is never dropped. When
adding another blocking read of an outside resource that Go cannot
interrupt, use the same shape (one owner goroutine, a buffered result,
leftover kept), not a goroutine per call, which would race on the reader
and lose data. Tests: TestSignalStopsReadLine and TestCancelStopsReadLine
in internal/gen/build_test.go (stdin kept open with a pipe; with no stdin
the program reads end of input at once and the test proves nothing).

Second trap (Observed 2026-10-09): the first version waited with its own
`tk.leave()`/`tk.enter()` around each of its two waits (the readers' turn
and the result), and `TestReplayEffects` failed: "diverged from
run.xotrace ... the trace's next decision runs main, which is blocked in
sleep". Record and replay count a blocking effect's leave and enter, so a
read must do exactly one pair, as the old direct read did; `stdinRead`
now leaves once for the whole read.
