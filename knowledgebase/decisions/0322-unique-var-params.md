# 0322: unique `var` List parameters skip the entry copy-on-write check

Status: Accepted (owner, 2026-10-21, via the lead: the Rust comparison's
ranked fixes; design reviewed by Xo lang 5, who owns insert_rc)

## Context

A function that updates a `var` List parameter starts with a copy-on-write
check: reuse_var_params (compiler/ssa rc.xo) puts a `reuse` of the
parameter at the entry, which loads the list's count and copies the list
when another reference holds it. bench/perf/llvm nbody calls
`advance(var bs: List[Body])` 5 million times with a list nothing else
holds. On the native backends the check is a load, a compare and a
branch a call. On LLVM it is also what keeps the bodies in memory: the
per-call count load and the copy call (which takes the list's address)
stop LLVM from promoting the five bodies to registers, which is how
Rust's build runs nbody in 1,410 M instructions against Xo's 2,780 M
(hand edited IR, ~/xo-lead notes nbody-llvm-2026-10-21: with this check
gone, list lengths given a range (the LLVM emitter's `!range !9`), and the
allocation's header visible, LLVM at O3 without polls runs nbody in
1,165 M).

insert_rc already proves lists unique inside a function (Rc.unique and
uniq: list.new and update results, reuse results, block parameters) and
then skips the check before an update. A parameter is never unique to
it: whether one is depends on every caller.

## Decision

An interprocedural analysis (compiler/ssa uniqvar.xo), run in rc_prepare
and rc_prepare_incr after the borrow inference and before insert_rc,
proves for each `var` List parameter whether every call passes a unique
list, as a fixpoint over the module like the borrow inference: optimistic,
each fact dropped on a counterexample until none changes.

- Candidates: a function whose name appears in the module only as the
  callee of a direct Call (no closure, function value, method table,
  thunk; not an entry, export, test target, or a function that catches
  or captures).
- A value is unique at a use when it comes from a unique origin, no other
  use of it can run after that use, every other use that can run before
  it only reads (list.len, list.index, list.get, a borrowed argument of a
  direct call), and the use's instruction takes it once. A use on a path
  that cannot reach the call may own it.
- Origins (closed: anything else is shared): list.new (unsized, it is
  the immortal empty list, which every update replaces or faults on
  before it writes, as insert_rc's uniq has it), list.set and list.push
  results, the list of list.pop and list.take, a reuse result, a
  candidate's unique `var` out read past the call's landing check, a
  candidate's unique List result, a unique parameter, and a Rebind or
  block parameter whose incoming values are unique. A Return in a fault
  exit (it calls rt.fault_continue) does not count: callers never read
  its outs.
- The entry `reuse` of a proved parameter stays, marked proved (aux 2):
  arm64 (and amd64 through it), LLVM, and wasm lower it to its argument.
  Removing it instead broke the module: the inliner's verifier, insert_rc's
  uniq, and the arm64 field and in-place passes all read a reuse result
  as the mark of an unaliased list (nbody built without inlining, +10.7%;
  gotchas/reuse-is-the-uniqueness-mark.md). The mark is part of every
  body the caches hash (rc_key's key_func, the backend's function
  keys), so no key needs a new line: a caller that changes a callee's
  fact changes the callee's body.
- XO_RC_CHECK=1 (Module.rc_check) marks it checked instead (aux 1): the
  check stays and, when the list is shared, calls the runtime's
  obj_rl_not_unique, which reports and exits with status 70 (the
  immortal empty list passes).
- XO_PASSES=-uniq_var_params turns the analysis off (compiler/arm64
  passes.xo).
- The analysis runs in rc_prepare (the full path) and, for the cached
  incremental path, in compiler/xo bcache.xo right after the borrow
  inference (phase insert_rc.uniq_var_params). Each function's uses and
  dominators are made only when needed, and the origin memo is one per
  function and round: 135 ms CPU in a self build (the first version,
  which made them for every function, about 365 ms).
- Interface dispatch never names a method as a value: its slots are
  Closure and Rebind values naming a generated thunk (ssa iface.xo),
  which calls the method directly with its own parameter, so the
  method's facts fall. (A `var` parameter through an interface is not on
  the native backends yet: XO1201.)

## Consequences

- nbody (Mac arm64, instructions retired): native 2,921 to 2,890 M
  (-1.1%), LLVM 2,781 to 2,765 M (-0.6%). The large LLVM result needs the
  allocation's header stores in the IR (the next step, C) and no
  preemption poll in the loop (Xo Lang 2, async preemption for LLVM).
- Every golden runs under XO_RC_CHECK=1 on arm64 (with XO_INLINE_STRICT=1)
  and LLVM; golden uniq_var_params covers the lists that must still copy
  (shared, kept in a struct, captured, iterated while passed, a constant
  literal) and two that are proved (threaded through var calls in a loop,
  recursive). A function value with a `var` parameter is not on the
  native backends yet (XO1201).
- The check mode caught two wrong proofs while this was built: a stale
  binary's (count IMMORTAL: the unsized list.new before the empty list's
  rule), and one list passed as two block parameters on one edge (`var q
  = t` threaded through a loop, golden noalias_lists, count 2), which the
  rule now rejects as a second use at the same point.
- The fixed point holds (scripts/gate.sh fp, s2 and s3 identical).
- Follow-up: the prelude's list_insert and list_remove_at outs are not
  proved unique, so their callers check again after them (done: the
  addendum below).
- Incremental builds: a caller flipping a callee's fact between unique and
  shared, and back, gives the same output with the caches on, off, and in
  verify mode, on arm64 and LLVM.

## Alternatives

- Removing the reuse: broke the passes that read it (above).
- Deciding inside insert_rc, where uniq lives: the callers' facts would
  depend on the callees' rc results, a cycle.
- A non-atomic count load in the check so that LLVM folds it (the
  experiment's first version): a race with another thread's decrement of
  a shared list; the lead ruled it out.

## Addendum (2026-10-21): what a callee hands back

The follow-up above: a caller's update of a `var` out or a List result
that its callee hands back unique (uniq_facts' out and ret: unique at
every Return but a fault exit's) gets its reuse marked like the entry
one, instead of checking again after the call.

- insert_rc's reuse step still inserts the reuse before the update (the
  mark every later pass reads, gotcha reuse-is-the-uniqueness-mark) and
  marks it proved (aux 2; checked, aux 1, under XO_RC_CHECK=1) when
  Rc.handed_unique holds: the updated leaf is the call itself (a List
  result) or one field of it (`.k`, the k-th `var` out), not a list
  inside one, not retained in the caller, its callee's fact true, and
  the update's block runs only past the call's landing check.
- One test of a call's field for both analyses: uniqvar.xo
  call_hands_unique (the fact and past_landing), used by origin and by
  Rc.handed_unique.
- past_landing finds a call's landing check by its pad, else the first
  rt.landing after it in its block that no value's pad names (the
  builder sets pad only on a block's last instruction; prune_landings
  removes a trivial check and clears the pad, and a later value's check
  in the same block is that value's: gotcha landing-of-another-value).
- The facts (Rc.uq, by function index) come from every function's body
  in every build (a candidate now needs a body: a function with none has
  no Return to disprove its facts) and are part of each caller's rc_key
  (the callee lines), so cached and cold builds mark the same reuses.
  No Func field and no cache entry format change.
- Why not drop the reuse: the first version made Rc.uniq true for such a
  field. The verifier after the arm64 inliner then failed compiler/ssa's
  shift_pos, where str.split had been inlined and its result was no
  longer a call (the build fell back to no inlining).

Verified (2026-10-21): golden uniq_call_outs (a caller's updates of a
handed back out and List result past the landing, and the same out
updated by deferred code on the fault path while an alias holds the
list the callee was given: it must copy) and uniq_var_params on arm64
and LLVM, plain and XO_RC_CHECK=1; compiler/ssa's xo tests (13/13, no
post-inline verification warning); internal/arm64 TestGoldenRun;
internal/llvm TestGolden with XO_RC_CHECK=1 (every golden: no checked
reuse found a shared list).
