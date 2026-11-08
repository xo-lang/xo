# 0562: Landings are made per function in insert_rc, not for the whole module

Status: Accepted (lead, delegated by the owner 2026-11-07), conditions below
(Xo Lang 4, 2026-11-07). A prototype (XO_LAZY_LANDINGS=1, uncached builds only)
is on branch l4-implland. Touches decision 0085 (landings and the unwind exit)
and decision 0382 (the cleanup cache).

## Context

Huge programs run out of memory to compile. Azure 1's GCP modules have about 170 MB
of linux/arm64 text, and a synthetic program of that size failed at 15.2 GB on the
offload host (exit 70, its memory budget). The measured program is g200: 2,000
generic instances of a 200 line arithmetic body, 13 MB of text. After 14cc2abc4
(cleanup in waves of 1024), its heap by phase on the Mac (`--phase-times`) is:

| phase        | heap after | peak     |
|--------------|-----------:|---------:|
| add_landings | 1,425 MB   | 1,425 MB |
| cleanup      | 1,921 MB   | 2,160 MB |
| insert_rc    | 1,684 MB   | 2,160 MB |
| emit         | 1,723 MB   | 2,160 MB |

That is about 140 heap bytes per byte of text, so 170 MB of text needs about 25 GB.
The heap profile at cleanup (XO_PROF_HEAP_AT=cleanup) has ssa.renumber's value
tables at 880 MB of the 1.32 GB in use: the SSA values themselves.

A census of the module (a debug count of values and blocks by op, branch
l4-census) shows where the values come from:

| point           | blocks    | values held | pads that only jump to the unwind exit |
|-----------------|----------:|------------:|------------:|
| before landings | 46,206    | 2,522,601   | 9,218       |
| after landings  | 2,463,652 | 6,148,955   | 1,217,887   |
| after cleanup   | 2,452,839 | 6,139,037   | 1,215,293   |
| after insert_rc | 19,611    | 2,489,005   | 0           |

add_landings (compiler/ssa/passes.xo land_at) ends the block after every operation
that can fault (each checked add, mul and rem) with `call rt.landing`, a branch to a
new pad block that jumps to the unwind exit, and a new block for the rest. On g200
that is 1.23 M calls, 1.23 M jumps and 1.21 M branches: the landings are 59% of the
values and 98% of the blocks, and cleanup removes about 0.2% of them.

insert_rc's rc_func needs them, because its liveness places the unwind releases on
the pad edges. At its end, prune_landings (compiler/ssa/rc.xo) removes every landing
whose pad does nothing but reach the unwind exit. It sets the operation's pad to -1
and merges the split blocks back. On g200 it removes all of them. Every backend
already compiles an operation that can fault with pad -1 as a fault to the unwind
exit: arm64 isel's cur_pad stays -1, and LLVM func.xo's pad_to stays -1.

So the landings live only from add_landings to each function's rc_func, the window
holding the compile's peak, and are almost all thrown away.

## Decision

add_landings no longer splits blocks. Instead it:

- marks each operation that can fault and has no landing with `PAD_LAZY` (-2) in
  Value.pi's pad field;
- makes the unwind exit of each function that has such an operation. The exit has
  to be made here, because its builder (finish_fn) can grow the module, which
  parallel jobs cannot do.

rc_func starts by making that function's landings (land_func: add_landings' block
walk for one function, moving each instruction once). The rest of rc_func is
unchanged, and prune_landings still removes the trivial ones at its end. At most
`jobs` functions hold their landings at once.

This is backend independent. What insert_rc hands the backends is the same shape as
today (pad -1 for a pruned landing, a real landing otherwise), so no backend changes.

- **Cleanup and renumber.** The readers between add_landings and rc_func (cleanup,
  renumber, the cleanup cache) see PAD_LAZY as "no landing yet". renumber already
  remaps only pads >= 0.
- **Cleanup cache.** The cleanup cache (decision 0382) stores bodies with the marks
  instead of the split blocks. The mode goes into cln_flags, so lazy and eager
  entries never mix. may_fault is already in its keys.
- **rc cache.** The rc cache (rccache.xo) is keyed by the body it is given. land_func
  is a pure function of that body, so the key still determines the result. Its
  encoder never sees PAD_LAZY, because rc_func makes the landings first.
- **Other callers.** The other add_landings callers (the `xo dev dump` pipelines, ssa
  main.xo) switch when the default does.
- **Verifier.** The post-rc verifier (verify.xo) runs after rc_func. An earlier check
  that finds PAD_LAZY reports a compiler bug.

### Addendum: conditions of acceptance (lead, 2026-11-07)

1. Built behind XO_LAZY_LANDINGS (classified in TestCodegenInputsClassified),
   default off until 3 and 4 hold. The cached path (cleanup cache) is covered
   before the flip, or shown never to see landings.
2. Correctness: every golden on arm64, LLVM, wasm and Go parity; the fixed point;
   `xo test` on std, native and LLVM; `xo dev codegen --backend=arm64,llvm`; and a
   fault and traceback set (fault_unwind, float_int_fault, the overflow traces)
   with tracebacks byte-identical to main.
3. Payoff, measured before the default flips: g200 peak RSS at least 30% lower,
   the self build's peak RSS lower, and compile wall time not worse (an s1ab or
   `--ab` self build, n >= 5, on linux/arm64 host 1: within noise or better). A
   cost from the serial unwind exit creation in add_landings that shows in
   parallel wall time is a fail. If this misses (for example a peak only 15%
   lower), the numbers go to the lead and the decision is revisited.
   Restated (lead, 2026-11-08), after the first host 1 measurement (one xo at
   l4-implland f80dbac8e, eager against XO_LAZY_LANDINGS=1, cold, cache off):
   g200 max RSS 2,349,172 KB to 910,428 KB (-61%, outputs identical), but the
   self build's max RSS did not move (4,210,640 KB to 4,210,728 KB). The self
   build's peak is set in insert_rc and emit (4.2 to 4.9 GB of heap), outside
   the landing window; its add_landings heap is only 1.35 GB, since ordinary
   code has few faulting operations per block. The clause now reads: the self
   build's peak RSS is not higher (the median of at least 5 within +1%), and
   compile wall time is not worse (an `--ab` self build, n >= 5 on host 1,
   within noise). In its place, peak RSS is measured on one real large
   generated module that motivated this (GCP compute/v0.alpha or armnetwork,
   from Azure 1's pins) and must show a real reduction.
4. The default flip is its own push under the default-change rule: the full
   suites of the touched packages, and a self build with the new xo
   (scripts/bootstrap-xo.sh s2) under XO_MAX_MEM=8G with --phase-times.

## Effect on memory and compile time

- **Memory (estimated, to be measured with the prototype).** Through cleanup g200
  holds about 2.5 M values and about 46 K blocks, instead of 6.1 M and 2.46 M. The
  value tables are the 880 MB at cleanup, and blocks, edges and land_rest shrink with
  them. Expected: the heap after add_landings goes from 1.43 GB toward the 0.55 GB of
  fe.build plus the marks, and the peak from 2.16 GB to about 1.0 to 1.2 GB (-45 to
  -55%). For 170 MB of text that is roughly 25 GB down to 12 to 14 GB. That fits the
  offload host's 15 GB budget, but not with much margin. Decision 0448 stage 3
  (storing bodies encoded) is the next step for the rest.
- **Compile time (estimated).** add_landings no longer allocates 2.4 M blocks, and
  cleanup walks a block list 50x shorter. The landings are still made, once per
  function inside rc_func's job, in parallel. g200's add_landings (0.76 to 0.92 s)
  and cleanup (0.26 to 0.42 s) phases should mostly go. The claim needs `xo bench
  --ab` (s1ab) on the self build and http_server_request. Ordinary programs have far
  fewer faulting operations per block, so expect within noise there.

## Risks

- **Unwinding and fault tracebacks.** Unchanged by construction. A fault's position
  comes from the operation's own lp, which the marks do not touch. The unwind path is
  the same landing rc_func would have seen, and the same pad -1 after prune. The
  risk is a reader between add_landings and rc_func that relied on the split, for
  example cleanup moving or merging code across a faulting operation, now that it is
  mid-block. A faulting operation keeps its order relative to effects within a block,
  as calls already do, but the prototype's goldens and the fault goldens
  (fault_unwind, chain_pads, conc_scopes, the catch goldens) are the evidence.
- **LLVM path.** LLVM func.xo and held.xo read pad_of only after insert_rc, and see
  the same pads as today. The LLVM goldens (TestGolden, TestGoldenGoParity) run in
  the gate. wasm and the Go backend are the same.
- **Output changes.** Cleanup on landing-free bodies can merge and reuse across what
  were separate blocks, so byte identity is not the check. The checks are the goldens
  on every backend, the fixed point, `xo test` on std, the codegen patterns, and
  TestGoldenRunIslands.
- **Cache entries.** Entries made under the old form are keyed apart by cln_flags.
  The cache tests (rccache, clncache) run in the gate.

Staged: XO_LAZY_LANDINGS (a classified codegen input) default off first; on after
the checks above; then the eager path is removed.

## Alternatives

- **Keep explicit landings and trim them in a late pass.** This is what happens
  today: prune_landings is that late pass, and it already removes all of them on
  g200. Trimming earlier, after cleanup, cannot work because rc_func's liveness
  needs them. Trimming in cleanup only saves the 0.2% cleanup removes. Rejected:
  the peak is before the trim.
- **Implicit landings in the backends, arm64 first.** A sentinel pad meaning "to the
  unwind exit" kept through every backend, read by the 75 pad_of readers in 26 files.
  arm64 isel would need one line (cur_pad = the exit's label); the SSA passes, inline
  and LLVM need the rest. Not needed: insert_rc already turns every trivial landing
  into pad -1, which the backends compile, so the backends never see the landings
  this ADR removes. An arm64-only stage would also split the pipeline by backend
  before insert_rc, which is backend independent today.
- **Both backends at once (implicit landings throughout).** Same as above with more
  sites, for no further memory gain: after insert_rc the census is already at 19.6 K
  blocks.
- **A smaller Value** (aux in one word, successors only on terminators). Fewer bytes
  per value, but it does not remove the 2.4x count, and it touches about 1,400
  sites. Worth doing after this.
- **Storing each function encoded between the front passes and insert_rc**
  (decision 0448 stage 3 item 1), or running insert_rc.apply through emit part by
  part. Both lower the peak independently of this and are complementary.
