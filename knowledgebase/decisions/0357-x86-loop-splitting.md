# 0357: split live ranges around inner loops on x86

Status: Rejected (measured 2026-10-23; the code is kept at refs/recover/x86-loop-split, c42454476, not on main)

## Context

JSON small on linux/amd64 runs 1.161 times LLVM's time. callgrind in
Docker amd64 (main f17591d26): 344.9M instructions, of which
`jsontext.scan` 107.0M and `prelude.llrt_json_find` 83.3M; LLVM's build
runs 300.1M, find 53.8M and scan 95.1M, so find holds 33.4M of the 48.5M
difference. find's member loop kept its counter, its bound, the tape's
base and length and `&tape[j]` in stack slots, while the key compare
loops inside it (range_eq, inlined) held the registers: the allocator
(compiler/arm64 regalloc.xo) gives a value one register or one slot for
its whole range, by weight 8^depth per use over its length, and x86 has
13 general registers.

## Decision

Not adopted. The design (lead-approved): before allocation on x86, a
value live through a loop that neither reads nor writes it, in a loop
whose pressure is over the class's registers, gets a new name across the
loop, with a never coalesced copy on the edge into the loop and copies
back on its exit edges (new edge blocks, compiler/arm64 x86split.xo).
The final version split only values a first allocation had spilled, then
allocated again.

Measured (callgrind Ir, the 12 JSON workloads, the same xo with the pass
off and on): every workload slower or equal: small 344.9M to 372.2M
(+7.9%), parse +14.9%, invalid +11.0%, maps +5.2%, floats +4.7%, ints
+4.0%, strings +2.2%, the encoders +0.0 to 0.9%. find 83.3M to 88.7M,
scan 107.0M to 127.7M. find's member loop itself got the registers it
wanted (no slot traffic for the counter, the bound or the bases), but
in this workload most members' keys have the key's length (name and
tags 4 bytes, email and score 5), so the inner loops are entered on most
members, and each entry and exit paid the split's copies (about ten
moves each way where the new names took other registers, a store and a
load each where they spilled). A split pays only when the loop it goes
around is entered rarely; nothing in the MIR says how often.

## Consequences

The allocator stays as it is. The cost in find is attacked instead by
making its inner loops cheaper or rarer: fewer live values in the key
compare (the key's data address rebuilt each byte, bounds checks on
both sides), and decoding an object in one pass over its members
instead of one find per field (derive Json's decoder: F fields scan F
members each).

Also measured and not adopted: x86 `add d, a, #0` as `movq` (585 to 74
`leaq 0(...)` in the JSON binary) and two consecutive leas as one: the
12 workloads moved by at most 405 instructions.

## Alternatives

- Static branch probabilities to place splits only around rarely
  entered loops: the compare's `raw == kn` is an equality test any
  heuristic calls unlikely, and it was likely here.
- A different allocator (second chance binpacking or graph colouring
  with splitting driven by a cost model): a larger change, worth it only
  with profile data or a cost model that sees loop entry counts.
- Rematerialization and check merging (the lead's options 1 and 3):
  covered in the design note; small here.
