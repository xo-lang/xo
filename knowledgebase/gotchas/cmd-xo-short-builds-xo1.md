# `go test -short ./cmd/xo` builds xo1 after every selfhost change

Status: Observed (2026-10-13, xo1-speed work)

Symptom: the repository's `go test -short` (about a minute otherwise)
failed with cmd/xo timing out at the default 600 s right after a merge
that changed selfhost/ and std; run alone, `go test -short ./cmd/xo`
passed in 428.7 s.

Cause: cmd/xo's tests run native builds through the self hosted driver
(decision 0178 switch point A), and the xo1 for the changed sources is
built first (minutes, once per change of selfhost/ or of a std module
xo1 uses since decision 0198). Under load that alone can take most of
the timeout.

Fix: after a selfhost change, run cmd/xo's short tests on their own
with `-timeout 30m` (the xo1 built then is cached for every later run
with the same sources), or with `XO_SELF_DRIVER=0` when the change is
not about the switch.
