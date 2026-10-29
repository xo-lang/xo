# Hand-written std/rt code reads an enum's Tag as 0 based

Status: Observed (2026-10-29, Xo Lang 2, Proc.notify on the Go backend)

## Symptom

A std/rt function that maps an Xo enum value to something by its `Tag`
picks the neighbouring case: `os.proc.notify([Signal.Hangup])` subscribed
SIGINT on the Go backend, so a SIGHUP still killed the program, while the
native backend (which matches on the variant) was right. A Go unit test
that built `Signal{Tag: 0}` by hand passed and hid it.

## Cause

The Go generator numbers an enum's variants from 1 (std_gen_*.go:
`case 1: "Hangup"`); Tag 0 is no variant.

## Fix

In hand-written std/rt code, index by `Tag - 1` (and build values with
`Tag: i + 1`), or switch on the generated constants. Unit tests build
values with the generator's tags, and a golden on the Go backend checks
the mapping end to end.
