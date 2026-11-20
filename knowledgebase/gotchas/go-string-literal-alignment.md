# Go string literal alignment moves byte search time

Status: Observed 2026-11-20 (task #467, bench/perf/gobridge bytes).

## Symptom

The Go backend's bytes row ran 1.32x its Go twin with the same algorithm.
The rfind loop over a `.bytes()` view of a string literal spent its extra
time in bytealg's head block. Separately, F_main's machine code was
byte for byte the same in two builds, yet its loop took 8.1 to 8.4 ms in
one and 6.2 to 6.5 ms in the other on linux/arm64 host 2. The only
difference was the `xo_gotimers` tag, which links one passive runtime
function (std/rt/gotimers_on.go).

## Cause

Go places string literal data with no alignment guarantee; a literal's
start can land anywhere in rodata, so arm64 bytealg's 32-byte vector
loops start with a partial block, and where it lands changes from build
to build. The second effect is code layout outside the loop: the loop
heads have the same offsets mod 32 and mod 64 in both builds, so it is
not loop head alignment. Host 2's container has no cache or branch
counters to show which.

## Fix

The Go backend hoists `.bytes()` on a plain literal of 32 bytes or more
into a package var `xo_alitN = rt.AlignedBytes(...)`, a copy on a 32-byte
boundary made once at init (cmd/xo TestGoBackendAlignedLiteralBytes).
Compare a timing across builds only when the hot function's layout is
known to match; a 30% change from an unrelated tag is layout, not the
change.
