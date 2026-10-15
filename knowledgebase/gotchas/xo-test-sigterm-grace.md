# A hung `xo test` takes 30 seconds to stop on SIGTERM

Status: Observed (2026-10-15, the zero-c regression sweep)

Symptom: hung `xo test` runs (cmd/xo's TestNoGCTests during the
0e1771fd hang) were still running seconds after `kill <PID>`, so they
looked as if they ignored SIGTERM. They were then stopped with
`kill -9`, which orphaned their test binaries and clang children.

Cause: none in xo. A test binary follows core 7.5's two phases. The
first SIGINT or SIGTERM only requests a stop. Cancellation comes at the
end of the grace period (30 s by default) or with a second signal, and a
third exits at once. Measured on main and zero-c, arm64 and LLVM, with
an endless test of `os.clock.sleep(1ms)` and with the real 0e1771fd
hang: one SIGTERM ends the run at 30 s, and two signals end it within
about 2 s ("Canceled", exit 1).

Fix: to stop a hung `xo test` you started, send SIGTERM twice to its
PID (a second `kill <PID>` a moment after the first), then wait.
Do not use kill -9: it skips the cleanup and leaves the test binary
running as an orphan.
