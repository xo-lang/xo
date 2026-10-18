# A new rt intrinsic in std/native needs the wasm runtime too

Status: Observed (2026-10-17)

## Symptom

After rt.mem_copy_block landed (ff426087), TestSelfhostWasm on main
passed 151 goldens against its floor of 167: 20 were "unsupported",
"the wasm backend does not support the call `rt.mem_copy_block` (stage
2), in libcstr.copy_words". The change's own checks (-short, goldens on
darwin/arm64, linux/arm64 with both backends, linux/amd64, darwin LLVM,
the fixed point) had all passed.

## Cause

The wasm backend compiles std/native/llrt too: libcstr.copy_words is
reachable from a wasm program. Grepping compiler/wasm for `libcstr`
finds nothing, which reads as "wasm never builds it"; the real list of
what wasm lowers is compiler/wasm/rt2.xo (the runtime functions, and the
direct list of names it calls). None of the native goldens runs wasm.

## Fix

Every new `rt.*` intrinsic declared in std/native/rt.xo gets a lowering
in each backend that can reach the caller: the arm64 (and amd64)
emitter, the LLVM emitter, and the wasm runtime (compiler/wasm/rt2.xo:
a function plus its name in the direct list; rt.mem_copy_block became
`memory.copy`, like $mem_copy). Before merging, run the wasm goldens
(TestSelfhostWasm, or `xo build --backend=wasm` of a golden that copies
a string) next to the native ones.
