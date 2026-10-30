# internal/gen's signal child outlived a killed test run

Status: Observed 2026-10-29.

Symptom: a compiled `signal` program (testdata/signal, parent PID 1, cwd
internal/gen) slept for 45 minutes after a gate was stopped.

Cause: startSignalChild killed its child only in a t.Cleanup. A gate stopped
by heavy.sh (the memory reaper) ends the test process without running
cleanups, and the child, blocked in a read, had nothing to end it.

Fix: the child runs under testtmp.Lifeline (a wrapper that ends it when the
test process dies, and after 5 minutes in any case).
