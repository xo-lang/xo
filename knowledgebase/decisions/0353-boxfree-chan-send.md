# 0353: Box-free channel send and receive for word-sized scalars

Status: Accepted (owner approved design, prototype and the full change through
the lead, 2026-10-23; Xo Lang 3; SSA split reviewed by Xo Lang 1). Landed in
13b1a2798: pingpong wall 0.835 and instructions 0.860 (`xo bench --ab`, 10
rounds, arm64).

## Context

`xo prof` of the 5M-iteration ping pong on main afc6ac09 (XO_PROF=cpu,
100 Hz, bench/perf/conc/xo/pingpong with n = 5,000,000) puts
`alloc.alloc_pop` at 11.6% of samples, called from `obj.try_pop`,
`obj.rl_grow_mt`, `cabi.obj_rl_grow_n` in main and the echo task. The
cause is the channel ABI: `Chan.send` lowers (compiler/ssa/conc.xo, the
`Chan.send` case) to `b.one(v)` (a `list.new` plus `list.push` of the
value, a heap list of one) and `rt.chan_send(h, l, loc)`; `Chan.recv`
calls `rt.chan_recv`, gets the list, and `pop_one` takes the element out
and frees the list. Every message therefore costs one list allocation and
one free, whatever the element type, and the value travels through the
runtime as the list's address (0 means closed in `chan.recv`).

The same profile shows `xo_stack_switch` at 27% flat, but it is 27
instructions: stalls, not work. Three switch changes were measured and
dropped (`ret` as `br x30`: wall 0.993; staggering the first frame against
4K aliasing: wall 1.014, instructions 1.000, both within noise; skipping
d8 to d15: 8 of about 2,000 instructions per handoff). The pingpong
comparison with Go on afc6ac09 (15 reps, load 5): wall 31.7 ms against
23.8 ms, CPU 30.0 against 31.6 ms, Mcycles 112.1 against 118.5. Xo does
less CPU work; Go's wall lead is its two goroutines overlapping on two
threads. The box is the one cost in the handoff path that is not
inherent.

## Decision (proposed)

A channel whose element type is a **word sized scalar that needs no drop**
sends and receives the word itself, with no list: the integer types
(Kind.Int: Int, the sized integers, Byte, Rune, converted with `IntConv`),
Float (its bits, `rt.float_bits` and `rt.float_from_bits`), Bool (0 or 1)
and enums none of whose variants has a field (the variant index, rebuilt
by a compare chain). Struct and Tuple never qualify: several words. Other
element types keep the list ABI unchanged (a later phase may take Str,
handles and one word payloads with a drop function). The handle reaches
the prelude functions as its address word (`rt.obj_addr`), because the
SSA verifier rejects a Chan[T] for an Int parameter.

1. **Where it lives (no new rt intrinsics).** The native prelude gets
   `llrt_chan_send_w(c, v, lp, ln)`, `llrt_chan_recv_w(c) -> (Int, Bool)`
   (value, ok), `llrt_chan_try_send_w(c, v) -> Bool` and
   `llrt_chan_try_recv_w(c) -> (Int, Bool, Bool)` (value, got, ok), thin
   wrappers over `std/native/llrt/chan.xo` functions that share the lock,
   ring, queues, `hand`, `det_point` and `sanhook` calls with the list
   versions. conc.xo emits them with `b.llrt_call` when
   `b.prelude_impl("llrt_chan_send_w") >= 0` (the native prelude; the
   probe `atomic_xo` and `sized_inline` use) and the element type
   qualifies. The wasm and Go backends have no such prelude function, so
   they keep the boxed lowering and their own channel implementations
   (compiler/wasm/rt3.xo `$chan_send`, std/rt) untouched. No backend
   emitter changes: `llrt_call` goes through ordinary calls, so the
   inliner can inline the fast path as it does for the other llrt
   functions.
2. **The channel record.** The ring slot and the waiter's `E_ITEM` are
   already one word, so the ring, the queues, `hand` and the lock stay as
   they are. Word mode is a property of the channel: `Chan.new` for a
   qualifying element type calls `llrt_chan_new_w(cap, lp, ln)`, which
   stores `CH_DROP = 1` (no drop function; `release` and `chan_free` skip
   the list release when it is 1). The record layout (the C `xo_chan`
   assertions, `CH_*`) does not change.
3. **Closed and zero.** The list ABI signals a closed channel with item 0.
   A scalar 0 is a value, so word receivers read a flag, not the item: a
   receiver woken by `close_body` gets `E_SHUT = 1` (the field senders
   already use for "closed while waiting"; unused for receivers), and
   `recv_w` returns `(item, E_SHUT == 0)`; the buffered path returns
   `(value, true)`, the drained closed path `(0, false)`. `recv` builds
   the Option from the ok flag in SSA (`Some`/`None` by a branch), as
   `pop_one` does now.
4. **ABI between boxed and word code (review by Xo Lang 1).** A channel
   stores one representation, and every list entry point converts at the
   runtime's boundary, so generic code and select cannot read a word
   channel as lists: `chan.send/recv/try_send/try_recv` test `CH_DROP == 1`
   and unbox (`unbox_word`: read the element, free the list) or box
   (`box_word`: a fresh list of one, the empty list for closed); in
   `selects.xo` a send arm unboxes when it is added, a recv arm on a word
   channel keeps the word with the closed flag in its entry's `E_SHUT`
   (`take` boxes), and `free` skips the list release. Select over integer
   channels is correct and no slower than today (it allocates the box it
   always did). Phase 2 may give select word arms.
5. **Unchanged semantics.** Blocking, cancellation points and unwinding
   (`b.check("")` after send and recv), the closed-channel fault text at
   `lp`/`ln`, `defer_check`, the deterministic scheduler's points
   (decision 0350) and TSan's happens-before calls run exactly as in the
   list versions; the word functions are those bodies with the item
   representation changed, not a second implementation of the queueing.
6. **Ownership.** A scalar has no reference count, so `rc.xo`'s
   `rt.chan_send => [owned, ...]` entry does not apply to the word path
   (there is nothing to consume); the boxed path is unchanged.

## SSA and backend split (to confirm with Xo Lang 1)

- Std (Xo Lang 3): `std/native/llrt/chan.xo`, `select.xo` boundary
  conversion, `std/native/prelude.xo` wrappers, `std/changes/prelude.md`.
- SSA (proposed Xo Lang 3, reviewed by Xo Lang 1, since ssa is theirs):
  the `Chan.new`, `Chan.send`, `Chan.recv`, `try_send`, `try_recv` cases of
  `compiler/ssa/conc.xo`, about 40 lines, choosing the word path through
  `llrt_call`; an `is_word_scalar(et)` predicate beside `drop_list`.
- Backends: none for phase 1 (arm64, LLVM, amd64 take ordinary calls). If
  Xo Lang 1 prefers the word ops as rt intrinsics so the emitters can
  inline them, that is a follow up measured with `--ab`.

## Plan and measurement

1. Prototype on arm64 for `Chan[Int]` only (no select), in a scratch
   worktree: `chan.xo` word functions, the prelude wrappers, the conc.xo
   cases. `xo bench --ab --rounds=10` on bench/perf/conc/xo/pingpong, plus
   fanout, mutex, spawn, rwmutex, pmap unchanged rows. The profile bounds
   the gain: `alloc_pop` is 11.6% of samples and the list free, the
   `rl_grow_n` call and its stores are more, so the expectation is a wall
   gain of 10 to 17% on ping pong and none elsewhere. If the prototype
   gains under 5%, the ADR is closed as not worth its surface.
2. Tests: a golden that sends and receives 0, negative values, the Int
   extremes, Bool, Float (NaN and -0.0 bit exact) and an enum, with
   buffered and unbuffered channels, `close` while receivers and senders
   wait, `try_send`/`try_recv`, a receive after close (None after the
   buffer drains, 0 is not None), a cancelled send, a `select` over a
   word and a boxed channel, and the deterministic runs (`xo test
   --explore`); all on arm64 and LLVM with Go parity (the Go backend runs
   the same program), the std corpora, and the fixed point (the compiler
   is built by Xo and sends on channels).
3. Phase 2 (separately measured): Str and handle elements with the
   existing drop function over the word ABI, select word arms.

## Result (2026-10-23)

Prototype on arm64, `Chan[Int]`, main afc6ac09, `xo bench --ab --rounds=10`:
pingpong wall 0.837 (0.813 to 0.852, better), instructions 0.856 (better);
fanout 0.976, mutex 0.944, rwmutex 1.005, pmap 0.992 (within noise), spawn
wall 0.941 (instructions within noise); the programs' outputs are
identical. The prototype also needed `for v in ch` (`for_chan`, a second
call site) and, per review, no Option on that loop (branch on ok). Golden
`chan_word` (0 and the Int extremes, close with buffered elements,
`try_*`, a select mixing send and recv arms, wake by send and by close, one
channel through send, recv, for and select; Int32, UInt8, Bool, Float
(zeros, a subnormal, a large value), an enum, a cancelled scope) matches
the boxed build's output on arm64, LLVM and the Go backend, and a test run
under `--explore=30` passes on arm64 and LLVM. The final build (all types):
pingpong wall 0.835 (0.817 to 0.856), instructions 0.860; fanout 1.022,
mutex 0.981, spawn 1.009, rwmutex 1.006, pmap 0.906 (30% spread), all
within noise.

## Consequences

- Ping pong and any scalar channel pipeline lose one allocation and one
  free per message; heap churn on worker threads falls with it.
- A second set of channel entry points to keep in step with the list
  versions (mitigated: shared helpers, the same golden on both).
- `CH_DROP = 1` is a new meaning of an existing word; chan_free and
  `release` must test it (both live in chan.xo).
- The Go backend, wasm and the C runtime path are unaffected.

## Alternatives

- **Cheaper box.** A per-thread cache of one-element lists in obj.xo:
  keeps the ABI, but the push, the stores and the free remain, and the
  profile's `alloc_pop` stall is the first touch of the block.
- **Value in the entry for unbuffered handoff only.** Faster still for
  rendezvous, but does not help buffered channels and needs the same
  word/closed split.
- **New rt intrinsics lowered by every emitter.** Lets the emitters
  inline the fast path, but touches arm64, LLVM, amd64, wasm and Go in one
  change; the prelude route gets most of it through the inliner and can
  be promoted later.
- **Stack switch changes.** Measured and dropped (Context).

## Phase 2: select word arms (2026-10-23)

A select over word channels no longer boxes: a send arm passes the word
(`llrt_select_send_w`) and a receive arm reads `(word, ok)`
(`llrt_select_take_w`), building the Option with `word_option`. The list
arms stay for other element types and for Wasm and Go. Selpp (ping-pong
through two select loops, arm64, `xo bench --ab --rounds=20`): wall 0.951
(0.925 to 0.975, better), instructions 0.982, max RSS 0.974. Str and handle
elements remain to be measured separately.
