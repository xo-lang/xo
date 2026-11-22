# dash's `.` passes no arguments to the sourced file

Status: Observed (2026-11-22, task #687)

## Symptom

`sh bench/perf/rust/run.sh` on host 1 (linux/arm64) stopped at once with
`bench/perf/xocheck.sh: 1: usage: . bench/perf/xocheck.sh XO`, although
run.sh sources it as `. "$root/bench/perf/xocheck.sh" "$xo" "$root"`.
The same script ran on the Mac.

## Cause

On the linux hosts /bin/sh is dash. POSIX leaves arguments to `.`
unspecified; bash sets them as the sourced file's positional parameters,
dash ignores them, so the file sees the caller's own `$1` and `$2` (empty
in run.sh). On macOS /bin/sh is bash in POSIX mode, so every Mac run
passed. A test that runs the sourced file under `dash -c '... . FILE A B'
sh A B` passes for the wrong reason: the caller's positional parameters
happen to be the same values; clear them with `set --` first.

## Fix

A sourced helper reads its inputs from variables the caller sets, or
falls back to them: xocheck.sh takes `${1:-$xo}` and `${2:-$root}`, the
names all nine of its callers use. bench/perf/xocheck_test.sh runs it
under dash with the caller's parameters cleared.

## Second linux trap in the same scripts: `cmd | awk || fallback`

bench/perf/twins/lib.sh's `load()` was `sysctl -n vm.loadavg | awk ... ||
awk ... /proc/loadavg`. A pipeline's status is its last command's, and awk
succeeds on empty input, so on linux (no vm.loadavg) the fallback never
ran: load printed nothing and every wait_load test said "Illegal number".
Pick the source with a test (`[ -r /proc/loadavg ]`) instead of chaining on
a pipeline's status. bench/perf/twins/lib_test.sh covers it.
