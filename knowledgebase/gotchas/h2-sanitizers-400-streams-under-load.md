# TestH2Sanitizers: "go 400 streams" drops below 400 under a full test run

Status: Observed (2026-10-10, during the real-time mode work, decision 0133)

Symptom: in `go test ./...`, `internal/llvm` TestH2Sanitizers/address and
/thread failed with `output lacks "go 400 streams: ok=400"` (the run
printed `ok=378`). Run alone, `go test -count=3 -run TestH2Sanitizers
./internal/llvm` passed every time.

Cause: not established. The 400 concurrent Go client streams against the
sanitized native server lose some under the CPU load of every package
testing in parallel (timeouts on the Go side are the likely suspect).

Fix: none yet; rerun the test alone before blaming a change. Next
experiment: run it with `-cpu 1` beside a CPU hog (`yes > /dev/null` on
every core, started and stopped by PID) and print the Go client's error
for each failed stream, to see whether it is a client timeout or a server
reset.
