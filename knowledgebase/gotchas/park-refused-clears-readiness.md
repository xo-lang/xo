# A refused park clears the readiness the caller then waits on

Status: Observed 2026-11-01

- Symptom: TestH2Diff, under load with more than one worker thread, left one raw case silent for 30 s (native `open` where Go answers GOAWAY, RST_STREAM or 200). XO_THREADS=1 and h2 parking off never did (Mac, 4 concurrent runs: 8 of 8 clean; default failed about 1 in 4 and once ran 79 s).
- Cause: hconn.park_ready calls poll.pd_clear_read, then MSG_PEEKs. Bytes that arrived between the h2 read's EAGAIN and the clear are found by the peek (the park is refused), but the readiness that announced them is gone; h2srv next() went straight to wait(), and the poller is edge triggered, so it slept until a deadline. The h1 caller reads on after a refused park and was not affected.
- Fix: next() re-reads once after a refused park (the `reread` flag) before it waits. Any caller of park_ready or hc_park that does not read after a false must do the same.
- Found with: XO_TRACE of a failing run (the stuck connection was a resumed task parked 30 s in `http`, max wake to run 3 ms: not the scheduler), then XO_THREADS=1 and parking off controls.
