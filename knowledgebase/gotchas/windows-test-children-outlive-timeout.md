# A Windows test's children outlive its timeout

Status: Observed (2026-11-10)

## Symptom

A winarm VM suite run passed its timeout: xo.test.exe (internal/arm64)
ended, and nine xo.exe processes it had started kept running for about
seven hours at full CPU (about 21000 CPU-seconds each, the VM at ~1200%),
until the lead stopped them on the owner's instruction.

## Cause

On unix, testtmp.Guard runs each child behind the lifeline wrapper, and
the child's process group ends when the test process does, however it
ends. On Windows Guard returned the command unchanged: there is no process
group, killing a process does not kill its children, and a `go test
-timeout` panic or an external kill of xo.test.exe runs no cleanup.

## Fix

internal/testtmp/job_windows.go: every test binary that imports testtmp
puts itself in a job object with JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE and
never closes the handle, so the job (every child and grandchild started
after) is killed when the test process ends. TestJobKillsChildren checks
it on Windows. To stop such a tree by hand: `taskkill /T /PID <the test's
PID>` on the process you started, never a name or pattern.
