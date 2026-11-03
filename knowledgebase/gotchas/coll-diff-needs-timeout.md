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

## Addendum (2026-11-02): a timeout was reported as a difference

Only the reference's timeout was guarded (REFFAIL); a native, LLVM or Go run that hit the 10 s limit under host load wrote `exit 124` and printed
`DIFF`, and KEEP copied only the program source, so the differing outputs
were lost. Five aliasbig "differences" of a 12000 program run (seeds 2645,
3047, 3048, 3133, 3159) did not reproduce alone, in 30 repeated runs, nor
in three 100 program batches rerun with RUN_TIMEOUT=60 under load.
coll-diff.sh now prints `TIMEOUT` (counted apart from `differ`) for any
backend's exit 124 or 137, and a DIFF keeps every backend's .out and .err
beside the source.
