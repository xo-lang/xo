# TestH2Diff's HPACK traces change when a case crosses a second

Status: Observed, fixed (2026-10-18).

## Symptom

TestH2Diff failed on both native backends with two hpack probe lines
different from the Go backend's, under a load of 12 to 22 (gate g70 of
xo-4b's combined branch):

    go:     ... 3[i8 i64 L28=R"51" L33=H"X"] ... 5[i8 i66 L28=R"52" i63]
    native: ... 3[i8 i64 L28=R"51" i63] ...    5[i8 i65 L28=R"52" i64]

## Cause

internal/llvm/hpack_probe_test.go shows each header field's
representation and masks the date's value as "X", but not whether it
came as a literal or an index. The server's Date changes once a second:
when two responses of a case fall in different seconds, the second one
is a new literal (L33) and every dynamic index after it moves by one;
in the same second it is an index into the dynamic table. Which side
crossed a boundary was timing, not code (here the Go side did).

## Fix

The probe cases start 20 ms after a second boundary and run again (up to
5 times) when the run ended in a later second than it started. Masking
the representation instead would hide the index shifts the encoder
probes exist to check.
