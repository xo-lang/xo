# A Go callback can run on another goroutine with the caller's task

Status: Resolved (2026-10-08, decision 0067; observed 2026-10-08 in the
arena audit of decision 0066)

Symptom: a handler that passed an Xo closure to Go
(`gosync.WaitGroup.new().go_(fn() { ... })`) got wrong results from
`m.values().sort()` in both the handler and the callback (36 to 48 wrong
responses and 67 to 96 wrong checks per run of `arena_holes`), only with
request scoped arenas on. After that was fixed, a callback that waited for
a Mutex its caller held crashed the program with a false `XO-F004:
Mutex.with called re-entrantly in one task` on Go's goroutine, and
`xo test --race` reported data races (golden `test_go_callback_task`).

Cause: the gobind adapter of a callback captured the caller's `tk` and
called `xoFn(tk, ...)`. Go may run the callback on another goroutine, while
the request runs or after it ended. The task carried the request's arena
(bump allocators for one goroutine), its thread record (scheduler turn,
recorded waits), and its held mutex set, all shared by two goroutines.

Fix: bindings of format version 3 (`gobind.Version`) wrap each callback
in `rt.NewGoCallback(tk)`, and each call in `cb.Begin()`/`cb.End(ctk)`. A
call on the caller's goroutine during a Go call of the caller runs as the
caller (without the arena); any other call is a child task with its own
thread, held set, and cancellation node (decision 0067). Older cached
bindings are regenerated. A Go call that blocks waiting for a callback
that waits for the caller's Mutex hung rather than being reported as a
deadlock (the caller is in Go code, which no monitor saw); since decision
0073 (2026-10-08) it is XO-F001 with the Go call edge when every
goroutine's stack proves it (`std/rt/godeadlock.go`).
