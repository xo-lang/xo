# 0204: The C runtime may call runtime functions compiled from Xo

Status: Accepted (owner, 2026-10-13). Supersedes decision 0180 item 3's
rule "C never calls compiled Xo".

## Context

Decision 0180 item 3 set a rule for the runtime port: a C function that
needs a ported function is ported with it, or the port waits; C never
calls compiled Xo, which would need an export mechanism. The zero-C
audit (bench/results/zero-c-audit-2026-10-13.md, "Owner decisions
found", C) found 1,806 lines of std/llrt "held by C callers" (number
text kept for fault messages, UTF-8 helpers, address policy, TLS name
checks), and that leaf pieces such as the allocator cannot move to Xo
first while C still calls them. Step 6 also had to make the C readers
pause at each parse and take the result back for the same reason.

## Decision

Allowed. Runtime functions written in Xo (std/native/llrt modules) may
be exported with the C ABI and called from the remaining C runtime.
The export is internal to the runtime (decision 0074): a marker or
table only std's implementation modules can use, never user code, and
no user visible language or spec change. Each export is a call
boundary like `use c` in the other direction: plain integers,
pointers to runtime objects and their reference counts follow the
runtime's existing conventions, and no Xo panic crosses into C (a
fault ends the program as today).

## Consequences

- The "(0) held by C callers" group (1,806 lines) can move now, and
  leaf pieces (the allocator, reference counts) can move before their
  C callers, which changes decision 0195's order where that helps.
- The xo1 emitters (arm64, LLVM, wasm) need a C ABI entry for an
  exported function; the Go side mirrors it only as the differential
  tests need.
- A mixed C and Xo runtime is harder to debug across the boundary;
  the C side shrinks with each step, and the exports go away with the
  C that calls them.

## Alternatives

- Keep 0180 item 3 (top down only): rejected by the owner; it holds
  1,806 lines and forces callbacks like step 6's pause and resume.
