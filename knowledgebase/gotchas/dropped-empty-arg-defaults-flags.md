# An empty argument dropped by an unquoted $args turned a sweep into one setting

Status: Observed 2026-11-06 (Xo Lang 2, the per connection memory sweep of decision 0520 phase 1).

## Symptom

A sweep of `harness load -sat 8`, `-sat 32`, `-sat 64` and `-sat 128` (each with `-rates ""`
to skip the open loop rates) printed four saturation rows with similar throughput and peak RSS
(59.8 to 71.8 MB), read as "memory does not follow connections". Every run had used 64 workers.

## Cause

The lead's wrapper (load-run.sh) passed its arguments on as an unquoted `$args`, so the empty
`""` vanished: the harness saw `-rates -sat 8`, took `-sat` as the rates value, and stopped at
`8`, its first non-flag argument (Go's flag package stops there without an error). Every flag
after it kept its default: `-sat 64`. The run looked normal, and its section was named after the
bad rates value.

## Fix

The wrapper passes `"$@"`; every harness subcommand exits 2 when arguments are left after its
flags (`unexpected argument "8"`). Write an empty value as one word (`-rates=`).

## How to avoid

A script that forwards arguments uses `"$@"`, never `$*` or an unquoted variable. A Go command
that takes no positional arguments checks `fs.NArg() == 0`. Before quoting a figure, check the
run log shows the setting the run was meant to have (here, the worker count in load.txt).
