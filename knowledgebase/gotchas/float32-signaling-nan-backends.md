# A Float32 signaling NaN keeps its payload on the Go backend and is quieted natively

Status: Observed (2026-11-06)

## Symptom

`TestStdCorpora/go/protobuf` failed on one `smsg` case while arm64, LLVM and wasm passed: the field-8 `Float32` written by
`derive Proto` was `0x7FAB6190` on the Go backend and `0x7FEB6190` on the others (Go's protobuf, through the oracle, wrote
the second).

## Cause (not traced into the backends' code; inferred from which bits differ)

`0x7FAB6190` is a signaling NaN (exponent all ones, top mantissa bit clear). The native backends probably keep a `Float32` in a
double register, so `Float32.from_bits(b).to_bits()` goes through a float32 to float64 to float32 conversion that sets
the quiet bit; the Go backend's `float32` value is never converted and keeps the bits. A quiet NaN, an infinity and every
other value agree on all backends.

## Fix

Do not use a signaling NaN as an expected value across backends: the protobuf corpus generator now sets the quiet bit of
a random Float32 NaN (scripts/stdcorpus/protobuf.go). Code that needs the exact bits of a signaling NaN must carry them
as a `UInt32`.
