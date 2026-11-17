# `xo bench --ab` on a server bench directory hangs

Status: Observed (2026-11-17, azure-1, task #99)

## Symptom

An `ab-host.sh` run of `xo bench --ab` on `bench/perf/httpproxy` printed
nothing for 15 minutes. Then OFF_IDLE_MIN stopped it and it exited with rc 125.

## Cause

`bench/perf/httpproxy` is a server: its main serves until something kills it,
so it never finishes and `--ab` waits forever. Directories like this are
measured by their twin's workload, not run as a bench.

## Fix

Measure such a module with `go run ./scripts/stdproven perf -rounds 9 MODULE`
(on host 1 through `stdproven-offload.sh` with `SP_EXCLUSIVE=1`). For httpproxy
it runs twincheck over `bench/perf/httpproxy_rules`. Before an `--ab`, check
that the directory's main returns on its own.
