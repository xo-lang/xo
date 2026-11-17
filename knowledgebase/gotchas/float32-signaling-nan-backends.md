# A Float32 signaling NaN keeps its payload on the Go backend and is quieted natively

Status: Observed (2026-11-06); fixed 2026-11-16 (#143)

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

## Traced and fixed (2026-11-16, task #143)

aws-43's amqp fuzz hit it again (0x7f800043 decoded as 0x7fc00043 on
amd64 and LLVM). The cause was in std/native/prelude.xo, not a backend:
every native backend holds a Float32 as its widened double, and
`float32_from_bits` built the exact double and then wrote
`Float32(Float.from_bits(d))`, a narrowing (`rt.f32_round`: fcvt on
arm64, fptrunc/fpext on LLVM) that quiets a signaling NaN;
`float32_to_bits` widened with `Float(f)` and set the quiet bit of any
NaN by hand. Both now reinterpret: `rt.f32_from_double_bits` and
`rt.f32_double_bits` (std/native/rt.xo), which the SSA builder lowers as
`rt.float_from_bits` and `rt.float_bits` typed Float32, so no backend
sees them (internal/repotest intrinsic_gaps.txt says so). Golden
float_nan_bits round trips signaling NaNs on arm64, LLVM, wasm and the
Go backend. Float (64 bit) already kept its bits. The protobuf corpus
generator's quiet bit (above) can go; a Float32 arithmetic result is
still a quiet NaN, as on the hardware.

How to avoid: a Float32 is a double inside the native backends; never
convert through `Float32(...)` where the bits must survive.


## The oracle quiets, not Xo (2026-11-17)

The generator's quiet bit cannot go after all: the protobuf oracle
(internal/stdtest/protoref) decodes into dynamicpb, which holds a float32
as a float64, so a signaling NaN comes back from Go quieted, while Go's
generated code and std/protobuf keep its bits. `stdproven fuzz protobuf`
seed 3 hit it on an unmarshal (sunmsg) input: field 8 `0xffa18e1c` was
written back as `0xffe18e1c` by the oracle. scripts/stdcorpus now quiets
a field-8 signaling NaN in sunmsg inputs too (protoQuietFl,
TestProtoQuietFl); the seed 0 corpus is unchanged.
