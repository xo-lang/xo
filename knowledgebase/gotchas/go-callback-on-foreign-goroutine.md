# A Go callback can run on another goroutine with the caller's task

Status: Observed (2026-10-08, arena audit of decision 0066)

Symptom: a handler that passed an Xo closure to Go
(`gosync.WaitGroup.new().go_(fn() { ... })`) got wrong results from
`m.values().sort()` in both the handler and the callback (36 to 48 wrong
responses and 67 to 96 wrong checks per run of `arena_holes`), only with
request scoped arenas on.

Cause: the gobind adapter of a callback captured the caller's `tk` and
called `xoFn(tk, ...)`. Go may run the callback on another goroutine, while
the request runs or after it ended. The request task carries the request's
arena, whose bump allocators assume one goroutine: two goroutines took the
same slots, and a late callback could allocate in an arena already reset
and handed to another request.

Fix: callbacks run with `rt.GoCallbackTask(tk)`, a copy of a request task
without the arena (any other task is passed as is); bindings of format
version 2 (`gobind.Version`) generate it, older cached bindings are
regenerated. The copy still shares the request's thread record and held
mutex map, as the callback did before; Xo code in a callback that blocks
or takes a Mutex from a foreign goroutine is not made safe by this.
