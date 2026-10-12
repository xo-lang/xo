# TestH2Diff "raw data over the window" can flake under heavy load

Status: Observed (2026-10-12).

Symptom: internal/llvm TestH2Diff fails on both native backends with one
line different: `raw data over the window: open` where the Go backend
prints `raw data over the window: 1[RST_STREAM FLOW_CONTROL_ERROR], open`.

Cause: seen once, in the combined full-suite run on dbb8b05 while five
agents and two heavy test runs shared the 12-core Mac (memory pressure,
load around 20). The same test passed twice in a row on the same commit
when rerun alone, so it is timing, not a code change: the raw client
reads the server's reply within a deadline, and the RST_STREAM came late.

Fix: rerun the test alone before bisecting (`go test -count=1 -run
TestH2Diff ./internal/llvm`). If it fails alone, it is real. A next step
to remove the flake: raise the raw client's read deadline for the
window cases, or have the test wait for the frame instead of a timeout.
