# LLVM compile time of wide values: stores, not literals (2026-11-08)

darwin/arm64 (the development Mac), clang -O1 on the module's whole IR
(`xo asm --backend=llvm`), base (f584f2f4b, unpatched) and new (cacc07cd0)
alternating, two rounds each; `xo build --backend=llvm`'s cc phase agrees.

## What the cost was

The request (Xo Lead, from Lang 1's poll): a 4,096 field literal took
12.1 s of clang against 0.5 s native. Measured again:

| Program | clang -O1 |
|---|---|
| hello world | 2.22 s (2.5 s under load later) |
| wbN, a 4,096 word struct of nested field defaults (wide_box without `==`) | 3.77 s |
| wbC (internal/arm64 wide_box: the same with `==`) | 6.78 s (7.0 s) |

The 12.1 s was the whole build's cc phase (std's code is most of it) and
predates 0a1b1e803. The literal's insertvalue chains were not the cost
(the constant aggregate experiment of wide-values-llvm-2026-11-08.md, and
again here: aliasing every all constant literal to one constant left wbN
at 3.94 s against 3.97 s). The cost was first class stores of the wide
value: wbC's main stored a whole `Node` (32,808 bytes) 8 times (into a
box, an optional's payload, a by reference argument's slot, the `==`
helper's arguments), and clang splits each into a store per scalar in
InstCombine and AArch64 instruction selection. Replacing those 8 stores
by hand with memcpys from a global took wbC from 7.0 to 2.85 s.

## The change (cacc07cd0)

compiler/llvm store_value: a struct, tuple or enum of 4 KiB or more
(WIDE_STORE_MIN, above every type xo itself has) is copied from the
memory that holds it, or written field by field when the function builds
it (a literal, a field set; a Field of either is looked through), and a
part of 256 bytes or more whose fields are all constants is copied from a
private constant global (`@.wc.N`). A wide variant is built in its memory
and keeps it as its address. Used where a value is written to memory:
Box, closure environments, by reference argument slots, the count and
equality helpers' pointer arguments, variant payloads.

| clang -O1 | base | new |
|---|---|---|
| hello world | 2.55 / 2.53 s | 2.53 / 2.55 s (IR byte identical) |
| wbN | 4.38 / 4.41 s | 3.94 / 3.98 s |
| wbC (wide_box) | 7.88 / 7.99 s | 3.96 / 3.95 s |
| golden wide_store (new) | 18.30 / 18.40 s | 6.78 / 6.74 s |

Field by field alone (no constant globals) moved wbC only to 6.75 s: the
scalar count is the same; the constant globals are the gain.

## What is left

wbN's main still costs 1.5 s of clang (wbN against wbN with main's body
emptied: 4.04 against 2.55 s): the Machine Instruction Scheduler
(+0.57 s), EarlyCSE (+0.45 s) and Machine Copy Propagation (+0.12 s).
Its non constant wide values (`b.core.s7.s7.s7.f7 = 77` is a chain of
extractvalue and insertvalue of whole L3, L2 and L1 values) live in
registers, which clang splits into thousands of virtual registers. Keeping
such values in memory is the by-value against by-pointer convention for
wide values (the Proposed ADR that follows this item).

## Identity and gates

The llvm gate step on linux/arm64 (host 2) builds every golden's IR at
the base and the change: 8 of 639 changed (amqp_tls_hostname,
amqptest_broker, big_struct, rc_empty_entry, rc_large_box_frame,
var_patterns, wide_capture, wide_store), and all 8 pass with the -O2
sample. Every other golden's IR is byte identical, as is hello world's.
xo's own types are all under WIDE_STORE_MIN, so the self build's IR is
unchanged; selfbuild 35 s, peak 4.8 GB. Codegen patterns: 38 programs,
298 patterns pass.
