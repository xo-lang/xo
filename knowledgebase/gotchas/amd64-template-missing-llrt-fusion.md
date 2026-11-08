# A new llrt function can need an amd64 template the arm64 build never shows

Status: Observed 2026-11-08

## Symptom

`xo build --backend=amd64` fails with `xo: the emitter skipped std/native/llrt/num.xo:148:34: in num.addw (inlined into num.append_json_float32): amd64 template: umaddl ...`, while every arm64 and LLVM build passes. Main was red on amd64 from dffd08f21 (2026-11-07) until 8a04478c8.

## Cause

The amd64 backend lowers the arm64 isel's output through templates in compiler/arm64/x86.xo. isel fuses a 32x32 multiply and an add into `umaddl` (decision 0208), and a new llrt function (dbox32's `addw(mulw(...), ...)`) was the first code to reach that fusion on x86. No test program built for amd64 used the function.

## Fix and guard

8a04478c8 added the `umaddl` and `smaddl` template. The gate's `cross` step (internal/arm64 TestAmd64CrossBuilds, cross builds for linux, windows and darwin on amd64, 2 to 6 s a program) now also runs when std/native/llrt changes, builds json_sized_ints, crypto_hash and tls12_client besides its first four programs, and its success is named `cross` in PUSHQ_RAN, which pushq requires for a change to compiler/arm64, compiler/ssa or std/native/llrt. Before the fix, the same build of json_sized_ints failed with the message above.
