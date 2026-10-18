# Go compiler audit of the arm64 backend (2026-10-18)

Owner's experiment: read Go's SSA rules and pass list
(`$(go env GOROOT)/src/cmd/compile/internal/ssa/_gen/ARM64.rules`,
`generic.rules`, `compile.go`), classify each rule group or pass as
have, missing, or n.a. for Xo's arm64 backend and SSA, rank the missing
ones by how often their shape executes in our own output, implement the
top ones, and compare the gain per hour with the differential method
(profile Xo against LLVM, fix the biggest gap). Ideas only; no Go code
was copied.

## Method

- Static: `xo asm --target=linux/arm64` of bench/perf/xo, the TLS
  client, the HTTP server, bench/perf/text and json (3,895 functions,
  757K instructions), shapes counted per function.
- Dynamic (the ranking): callgrind `--dump-instr=yes` on Linux arm64
  (Docker), per instruction self cost joined with `objdump -d`, over the
  18 bench/perf/xo workloads (8.9G instructions executed) and 25 TLS
  handshakes (p256, x25519, tls12, rsa2048, ed25519; 153M). Main at
  96e0a477.

## Classification

| Go rule group or pass | Xo arm64 | Notes |
|---|---|---|
| ARM64.rules: compare with zero to cbz/cbnz | have | isel fuses `cmp #0; b.eq/ne`; leftovers are runtime assembly |
| ARM64.rules: sign test to tbz/tbnz #63 | missing, done | `cmp #0; b.lt/ge` was two instructions |
| ARM64.rules: single bit test to tbz/tbnz | have | `tst_bit` |
| ARM64.rules: zero register stores (`MOVDstorezero`) | have (mostly) | 0.02% of TLS executed |
| ARM64.rules: bitfield ops (ubfx, ubfiz, sbfiz, bfi) | missing | never emitted; 0.16% of TLS, 0.03% of xo executed |
| ARM64.rules: madd/msub, shifted operands, shifted index loads | have | 543, 2,486, 468 static uses |
| ARM64.rules: csel/cset/csinc from phis | have | isel select_branch, select_compares |
| ARM64.rules: load/store pairs | have | lsr.xo pair_mem |
| ARM64.rules: logical with inverted operand (bic, orn, eon) | have | ea947cfc |
| generic.rules: constant folding, strength reduction | have | fold_consts, mul by power of two, magic division |
| generic.rules: StructSelect of StructMake (field of a tuple) | missing | the inliner's return tuples; see next steps |
| pass: cse (global) | missing, done | SSA cse after inlining |
| pass: shortcircuit / branch threading on phi constants | partial, done | MIR thread_const extended |
| pass: prove (bounds and overflow facts) | partial | ranges in isel; unsigned bounds compares are 4.7% of xo executed, not all removable |
| pass: loopRotate | partial | some loops rotated by threading; backward `b` is 1.87% of xo executed |
| pass: tighten, layout by likely | partial | cold blocks; no likely-based layout |
| pass: dse | missing | 0 static candidates found |
| pass: memcombine | missing | not counted |
| pass: sccp | missing | fold_consts covers the local cases |
| pass: regalloc (spill placement) | partial | 1.57% of xo executed is a store back of a value loaded from the same slot (Xo Lang 2's area) |
| pass: nilcheck, writebarrier, race | n.a. | no nil, no GC write barrier |

## Dynamic evidence (share of executed instructions)

| Shape | bench/perf/xo | TLS client | Go counterpart |
|---|---|---|---|
| register move (`mov reg, reg`) | 7.19% | 3.22% | regalloc |
| unsigned compare and branch (bounds checks) | 4.71% | 0.21% | prove |
| unconditional `b` | 2.80% | 0.50% | layout |
| backward `b` (loop not rotated) | 1.87% | 0.25% | loopRotate |
| store back of a value loaded from the same slot | 1.57% | 0.00% | regalloc spill placement |
| add recomputed after the overflow checked adds | 1.12% | 0.00% | cse |
| `cmp #0; b.lt/ge` | 0.89% | 0.08% | tbz rule |
| branch on a constant just set (2 to 3 saved each) | 0.42% | 0.00% | shortcircuit |
| `lsr; and` (ubfx) | 0.00% | 0.11% | bitfield rules |
| `cmp #0; b.eq/ne` | 0.02% | 0.04% | cbz rule |
| `mov #0; str` | 0.00% | 0.02% | zero register stores |

Concentration: the backward `b` is mostly list.remove_at (insrem), the
store backs are maxby's loop, the constant branches are sortpt's derived
Ord compare (`mov x9, #-1; cmp x9, #0; b.lt`).

## Implemented (branch xo4-audit2)

| Item | Source | Result |
|---|---|---|
| tbz/tbnz #63 for a sign test | Go rule | together with thread_const: portable/u64 0.937, unix 0.947, conv 0.966 |
| thread_const: constants set in another block or by a move of an immediate, to a fixpoint | Go shortcircuit | constant branches in bench/perf/xo 118 to 47; Linux xo/sortpt 0.868 (its binary search tests the compare's flags directly) |
| SSA cse after inlining | Go cse | remove_at's `j + 1` once instead of three times; bench/perf/xo assembly -1.6% |
| frame homes and field homes for large `var` arguments, field read in place | differential (Xo Lang 3's HTTP/2 profile) | bench/perf/varparam 522,738,826 to 33,738,967 instructions on Linux (LLVM 21 a call); HTTP TLS request -20.7%, h2 request -22.8% |

Three miscompiles were found while gating, all fixed on the branch
before any push:
- threading a jump past a loop header from outside gave the loop two
  entries, and hoist_consts' natural loop of it then reached another
  loop (tlsname.ip_string read a register no path wrote);
- cse removed an operation that had a landing pad, so the inlined
  module failed its reference count check and was built without
  inlining;
- a frame home holding a loop's parameter was written only by the back
  edge, so a loop that ran zero times passed an unwritten home to the
  next call (`xo publish` segfaulted; golden byref_home_loop).

## Gate (g23, branch on ac908f21 against ac908f21, 2026-10-18)

- Fixed point, `-short`, internal/llvm (golden, leak check, TLS interop,
  tasks), internal/arm64, internal/gen properties (400), cmd/xo leak
  test: all pass on macOS arm64. Linux arm64 goldens: 191 ok in Docker.
- Mac sweep (114 workloads, 3 rounds): 37 faster beyond spread, 3
  slower. Faster: httpparse/h2_varied 0.895, h2_get 0.905, tls/p521
  0.877, portable/u64 0.937, unix 0.947, httpclient/h2big 0.960,
  rsa/sign2048 0.963, portable/conv 0.966, json/ints 0.972. Slower:
  portable/divmod 1.026, httpclient/big 1.021 (spread 1.020),
  digest/none 1.015.
- __text: hello 364,240 to 358,008 bytes, bench/perf/xo 1,927,812 to
  1,782,320 (-7.5%), the TLS client 1,586,992 to 1,538,836.
- Linux callgrind, bench/perf/xo: sortpt 0.868, get 0.974, putget 0.983,
  copymut 0.984, update 0.985, the rest 0.996 to 1.000 (insrem 1.000:
  loopRotate is still open). TLS handshakes 0.993 to 0.997.
- HTTP server callgrind (instructions a request, Linux arm64):

| Route | main | branch | ratio |
|---|---|---|---|
| plain | 31,012 | 30,769 | 0.992 |
| plain, new connection | 39,997 | 39,699 | 0.993 |
| tls | 73,782 | 58,533 | 0.793 |
| tls, new connection | 2,083,926 | 2,055,943 | 0.987 |
| h2 | 125,633 | 96,949 | 0.772 |

## Method comparison

- Differential fixes, 2026-10-17: the address fold plus the ELF label
  fix made all 18 Linux bench/perf/xo workloads faster (branch 0.534,
  copyset 0.656) and TLS handshakes about 4% cheaper, in about 3 hours;
  agg_rc_at made the HTTP TLS request about 3% cheaper.
- Go audit items (tbz, thread_const, cse): about 2 hours to classify
  and weight plus about 4 hours to implement and fix two miscompiles.
  They gave 1% to 6% on the integer and parsing workloads, 13% on
  sortpt, and -7.5% code size together with the rest. The rules with the
  clearest Go counterparts (cbz, zero stores, bitfields) barely execute
  in our output; the shapes that do execute (register moves, bounds
  checks, layout, spills) are passes, not rules, and are larger projects.
- The profile driven item on the same branch (var homes, about 4 hours
  including the third miscompile) gave the largest wins: 15x on its
  shape, 21% and 23% of the TLS and h2 request.

## Recommendation

Keep the differential method (profile Xo against LLVM, fix the largest
gap) as the driver, and use the dynamic shape table above as its
checklist: it says which Go passes are worth porting (regalloc spill
placement, prove, loopRotate) and which Go rules are not (bitfields,
zero stores). Read Go's rules for a shape only once a profile points at
it.

## Next steps

- Field of a tuple fold plus nested in-place set fields across one edge,
  so tlssrv.flush's TLS 1.3 path stops copying `c.s` (flush is 3,043
  instructions with inlining, against 3,667 before the branch; LLVM's
  whole call is about 200).
- portable/divmod 1.026: find which of the three items costs it.
- loopRotate for the remaining backward branches (insrem).
- Spill placement in maxby's loop, with Xo Lang 2 (regalloc).
- Bitfield rules only if a workload needs them.
