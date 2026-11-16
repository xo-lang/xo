# `xo test --std-module=NAME` is ambiguous when std/native has NAME too

Status: Observed (Xo Lang 3, 2026-11-15)

## Symptom

A std push's gate failed in its stdfast step with no failing test:
`xo test: --std-module=cbor is ambiguous: native/cbor or std/cbor`
(stdfast cbor: FAIL, the step's log one line long).

## Cause

gate.sh's stdfast step passed the bare module name that gatelib's
gate_stdfast_mods prints. A std module with a std/native module of the
same name (cbor, and any later pair) makes the bare name ambiguous for
`xo test`. `xo vet --cost --std-module=cbor` still resolves it.

## Fix

Three callers passed the bare name; each now passes `std/NAME`: gate.sh's
stdfast step (gatelib gate_stdfast_test_mod, a gatesteps_test.sh case),
its wasm std loop (when std/native/NAME.xo exists), and internal/stdtest
TestStdModules (stdModuleArg, with TestStdModuleArg), which failed next
as `TestStdModules/arm64/cbor`. By
hand, run `xo test --std-module=std/cbor` and
`xo test --std-module=native/cbor`.
