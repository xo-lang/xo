# TestRuntimeIntrinsicsLowered reads any quoted runtime name as a lowering

Status: Observed (2026-10-28, 0a58e8e53)

Symptom: internal/repotest TestRuntimeIntrinsicsLowered fails with
"intrinsic_gaps.txt lists `arm64 cast` but the arm64 backend lowers rt.cast
now: remove the line", after a change that lowers nothing new.

Cause: the test decides that a backend lowers rt.N when its sources
contain the quoted text "rt.N" or "N" anywhere. compiler/arm64/stacksmall.xo
(decision 0395) named rt.cast, and a list of read-only runtime calls, as
quoted literals to recognize them, which the test took as lowerings.
Removing the gap line would have hidden a real gap.

Fix: in a backend's sources, name runtime calls you only recognize
without quoting them: stacksmall.xo's rt_named(name, " cast str_view ")
matches against a space-separated word list. Then run
`go test ./internal/repotest` (it is one of pushq's fast checks only when
PUSHQ runs them; a push without its gate skips it).
