# A stopped `spin up` keeps serving the old component

Status: Observed (2026-11-03, decision 0447 stage 5 step 1)

Symptom: a new WASI 0.3 component answered 500 under Spin 4.2.6 on every
request, run after run, while `wasmtime serve` served it. Spin's log
said `127.0.0.1:PORT is already in use`.

Cause: the script started Spin as `(cd app && spin up --listen ADDR) &
sp=$!` and stopped it with `kill $sp`. That stops the subshell only.
`spin up` runs its trigger as a child process, which kept the port and
the component it had loaded. Each later run's Spin failed to bind, and
the curl requests reached the earlier run's trigger, still serving an
older, broken build.

Fix: start the server in its own session and stop that session's
process group by the PID the script captured: `(cd app && exec setsid
spin up ...) & sp=$!`, then `kill -- -$sp`. Use a port that differs per
run (from `$$`), so a leftover cannot answer for a new run. Check the
log for "already in use" before trusting a Spin result. The Go tests
already do this: TestWasmComponentServe stops the group through
testtmp.Lifeline. Never stop such leftovers by name or pattern.
