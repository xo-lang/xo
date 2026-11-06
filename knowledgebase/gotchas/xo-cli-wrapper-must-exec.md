# An XO_CLI wrapper that runs xo as a bash child hangs cmd/xo's tests

Status: Observed (2026-11-06, linux/arm64 host 1 and the linux/amd64 host, xo-lang-4)

## Symptom

A suite run with XO_CLI pointing at a counting wrapper (a bash script
that times each xo call and appends a line to a log) never finished:
`go test -short` over the repo stopped making progress after 1906 xo
calls (offload.sh's no-progress check stopped it after 15 minutes, with
an empty log), and two full `go test ./cmd/xo` runs ran 1 h 50 min and
2 h 6 min without ending until stopped. The same suites with xo itself,
or with a wrapper that ends in `exec xo`, finish (-short in 121 s).

## Cause

Likely (not traced to the one test): the wrapper ran xo as its child
(`"$REAL" "$@"; rc=$?`) to time it, so the process a test started was
bash, not xo. cmd/xo's server tests (xo serve, xo lsp, a run they stop)
signal, kill or wait on the process they started and its pipes; bash
waiting on a foreground child does not pass the signal on, so xo kept
running and holding the pipes, and the test waited. Without `go test
-v` the log showed nothing, so the stall looked like a long suite.

## Fix

A wrapper for XO_CLI writes what it records first and then `exec`s xo,
so the test's process is xo itself (itemc-suites-short-remote.sh
--exec, itemc-amd64-remote.sh). Timing per call then needs another
source (--phase-times, XO_CACHE_STATS); count calls with the wrapper.
Run long offload suites with `go test -v` (or -json) so a stalled test
shows its name and the time it started in the log.

## How to avoid

Never put a non-exec shell between a test and the process it manages.
