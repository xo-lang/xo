# coll-diff.sh on macOS compared every program as the same

Status: Observed (2026-11-02, checking the Go only golden differences)

Symptom: `REF=llvm scripts/coll-diff.sh bin/xo GOLDEN...` on the Mac
printed `same` for native_sql, whose Go and LLVM outputs differ on a
line (Go has no Postgres driver, decision 0278 point 9).

Cause: the script runs each program under `timeout`, which macOS does not
ship. Every run printed `command not found` to its stderr and wrote
`exit 127` as its whole output, so all backends agreed.

Fix: coll-diff.sh now stops with exit 2 when `timeout` is missing. Run it
on the linux offload hosts (they have coreutils), or install coreutils on
the Mac. A comparison tool that wraps runs in a helper should check that
the helper exists, and a sweep where every program agrees deserves one
known differing program as a control.
