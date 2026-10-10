# Native error frames of closures had the function's name

Status: Observed (2026-10-09, comparing os_tasks's report)

Symptom: a `?` inside a closure printed `  at ..run(k=1)` natively and
`  at ..run.closure(k=1)` on the Go backend; a `?` inside a closure given
to `Mutex.with` printed no frame natively.

Cause: decision 0104 recorded the enclosing function's frame for a `?` in
a closure but not the `.closure` suffix the Go backend adds once per
closure level; `closureWith` (Mutex.with) set no frame owner.

Fix: `fn.closures` counts closure levels, `pushFrame` appends `.closure`
per level, and `closureWith` records the owner's frame and captures its
parameters. Golden internal/gen `error_frames_closures` in
TestErrorTracesNative.
