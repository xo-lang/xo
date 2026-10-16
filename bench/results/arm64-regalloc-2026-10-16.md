# The arm64 register allocator in long straight blocks (2026-10-16)

xo-fa, for the lead's follow-up to arm64-inline-2026-10-15.md ("the root
lever: the arm64 register allocator in long straight blocks"). Time box:
two attempts. Neither pays on current main; the code is not merged. It
is on the branch `fa-regalloc-attempt` (21ff4583) for reference.

## Question

selfhost/arm64 regalloc.xo is a greedy first fit over whole live ranges,
with no splitting. In the crypto field arithmetic (nistec mul4p, x25519
fe64 mul and square, Keccak rounds) it spills where LLVM does not. Which
of its decisions differs, and can a fix pay with no regression on the
arm64 bench set?

## Diagnosis

Stack loads and stores per function, arm64 against LLVM (`xo asm`):

| Function | arm64 loads / stores | LLVM |
|---|---|---|
| nistec.mul4p | 19 / 11 | 163 instructions in all (arm64 172) |
| nistec.mul6 | 83 loads | |
| sha3.keccak | 373 loads | 107 loads |
| x25519.scalar_mult | 89 loads | 29 loads |

The difference in mul4p and mul6: the source (and hoist_consts) loads
every limb of both operands at the top of the block, so the early
definitions live across the whole block and are what the allocator
spills. LLVM's scheduler sinks each load to its use. Two things stopped
a sink in our MIR: an inlined call's continuation is its own block
(inline.xo), and the call's arguments and result pass through a
parallel move.

Keccak and x25519 are different: 25 lanes plus the round temporaries
are live at once, more than the registers, in any order. Sinking does
not touch that; it needs splitting (or rematerialization from the state
array).

## Attempt 1: sink loads to their use

New pass file sink.xo, run before regalloc:

- merge_straight: join a block that ends in a jump to a block only it
  enters (same temperature, not the entry or a landing pad);
- fold_moves: drop a parallel move inside a block when both sides are
  defined once (the inlined call's arguments), renaming the readers;
- sink_loads: move a plain load or constant down to just before its
  first reader in the block, past nothing that writes memory or its
  registers.

mul4p went from 19 / 11 stack loads and stores to 6 / 8 (172 to 162
instructions, LLVM 163); mul6 from 83 to 37 loads. Keccak and
x25519.scalar_mult unchanged.

First gate run: programs hung (recursion's eval). merge_straight had
joined shrink wrapping's restore blocks to their predecessors, so a
return jumped to the shared `ret` without popping the frame. Fixed by
not merging across the shrink region (shrink.xo region and fedge);
gotchas/arm64-merge-across-shrink-boundary.md. Then every gate passed
(goldens, devgoldens, strict inlining codegen, self build then hello,
fixed point).

Sweep, 151 arm64 workloads, 3 reps (main at 9c88e3f0): 8 faster, 36
slower beyond spread. TLS p384 0.978, argon2 0.986; json, text, alloc,
stdio 1.003 to 1.017; fault isolate 1.392, fault owned 1.179.

## Attempt 2: only in long straight code, and keep the cheaper allocation

- 1b: the passes act only on a chain of 48 instructions or more
  (LONG_STRAIGHT), so other functions compile as before. Sweep: 5
  faster, 2 slower. p384 0.969, p256 0.985, x25519 0.988, argon2
  0.985; text hex 1.006 (num.from_hex: sinking lengthened other ranges
  and the allocator saved two more callee saved pairs, four
  instructions on every call), fsproc remove 1.003 (no function
  changed: noise).
- 1c: a function the passes change is also allocated without them, and
  the allocation with the lower cost is kept (spilled reads and writes
  in hot blocks, plus two per saved callee saved register). Sweep on
  9c88e3f0: 0 slower, 4 faster (argon2 0.988, p256 0.985, x25519
  0.986; p384 0.968 from the two reps that ran, the third lost its
  sockets to port exhaustion on both sides). Compile time of
  selfhost/xo (cache off): user 118.15 s to 144.97 s, real 35.33 s to
  38.98 s.
- 1c with a cheap pre-check (copy and allocate twice only when a long
  chain exists; skip when the plain allocation spills nothing hot),
  rebased on main 370c7ae3, where adc chains are on again (bc2068c8, up
  to 8 pairs an instruction): 2 faster, 2 slower. p384 0.981, argon2
  0.987; tls ed25519 1.002, tls12 1.004; p256 and x25519 no longer
  faster. Compile time: user 140.08 s to 164.90 s (+18%), real 33.30 s
  to 37.14 s (+12%).

Gates: 1c passed all 179 goldens on linux/arm64 in Docker (before the
pre-check and the rebase); the final version passed internal/arm64 (99
s), strict inlining codegen, self build then hello, and the fixed point
on the Mac. (The Mac's devgoldens runner times out on six
network goldens at the time of writing with main's compiler too: an
environment issue, not this change.)

## Why it does not pay

The adc chains took most of the gain: each pair is one instruction with
its operands named in the template, so the limb loads now sit at their
use already, and what the passes still move is a few loads. What is
left (p384 2%, argon2 1%) does not cover 18% more compile CPU and two
small TLS regressions.

## Next levers, ranked by likelihood of paying

1. bic, orn, eon selection for (~x & y), (~x | y), (~x ^ y): Keccak's
   chi is 100 mvn plus and per call (Xo lang 4's callgrind), and each
   mvn is also a live temporary. Cheap, no compile time cost. Then
   measure Keccak's spills again.
2. More caller saved registers: the emitter reserves x15, x16, x17 as
   scratch, leaving 15 against LLVM's 18. Since 4bf54541, scratch_reg
   can take a free register at the instruction, so x15 can join the
   allocator's pool (and x17 when no template reloads two spilled
   operands). Measure p384, x25519, keccak and the self build.
3. Spilling Keccak's lanes to their home in the state array instead of
   a stack slot (rematerialize by load, no store back until the round
   ends): the 373 loads against LLVM's 107 are mostly reloads of lanes
   the round reads twice.
4. Live range splitting at the block boundary for loops over large
   state (x25519 ladder, Keccak): a real allocator change, and the only
   one that reaches the remaining gap; a second allocator (linear scan
   with splitting) for functions over a size, compared by alloc_cost as
   attempt 1c does, keeps the rest unchanged.
