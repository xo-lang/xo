# The wasm build of the checker overflows a browser's stack on a long operator chain

Status: Observed (2026-10-16, xo-4b, the /play-next/ playground).

## Symptom

The compiler built for wasm (selfhost/wasm, decision 0242) stopped in a
Chrome worker with `Maximum call stack size exceeded` while checking any
program in native mode; under Node with a 1 GiB stack it worked, and
with `XO_WASM_STACK_MB=4` it failed too (16 passed).

## Cause

std/native/llrt/tzhost.xo built a constant from 138 strings joined with
`+`. The parser nests such a chain to the left, and the checker recursed
`infer -> infer0 -> binary` once per term. Each level's three wasm frames
held about 2,700 parameters and locals (the Checker passed as 45 leaves,
infer0 1,407 locals), about 21 KB of V8 baseline stack per level, and a
browser worker allows about 0.6 MB (gotchas/wasm-browser-stack-locals.md).

## Fix

The checker walks a left-nested chain of operators in a loop
(selfhost/check expr.xo infer_left, with the recursive walk's types and
diagnostics), and the constant is one triple-quoted literal. To find
such a cycle: run the module under Node with a small
`XO_WASM_STACK_MB` and `--stack-trace-limit`, and map the `fN` frames
to names with the annotated .wat.
