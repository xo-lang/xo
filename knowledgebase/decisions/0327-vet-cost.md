# 0327: `xo vet --cost`, notes on costs the source does not show

Status: Accepted (owner, 2026-10-21, from the agents' language poll; design
reviewed by the lead). Implemented: XO0907 (stage 1), XO0908, XO0910.
XO0909 claimed, to follow (Xo Lang 3).

## Context

On 2026-10-21 a loop over a store's map under a read lock
(bench/perf/http/xo store.xo, `for (_, u) in st.users`) copied every
entry first, a dup and drop of every string per GET, counted atomically
by every core (decision 0291 addendum). Nothing in the source, the
checker, or `xo vet` showed it: the cost was in what the compiler made of
the loop. Agents write most of the code and read diagnostics, so a cost
they cannot see is a cost they keep.

## Decision

`xo vet --cost` adds advisory notes (never errors) on costs the compiled
program has and its source does not show. vet (compiler/cli/core) has
the checker only, so the driver (compiler/xo vetcost.xo) handles `vet
... --cost`: it checks the program as `xo build` would for the host
(silently: a program that does not check gets no cost notes; vet reports
its errors), builds it up to insert_rc (no inlining, code, or link), and
compiler/ssa cost.xo reads the root module's functions. The notes join
vet's findings (core.run_extra), in the same text and JSON forms.

Notes (spec section 10), each about a loop unless said:

- XO0907 (stage 1): a loop that copies every entry of a map before it
  starts: a call to xo.maph.logkvl or xo.mlog.each whose result a loop
  reads, when the loop's line has no in-place walk (xo.mlog.eachpos):
  a log-mode map through a forked view, or a body the walk does not
  take (over 64 nodes, a closure, a defer).
- XO0908: in a loop, a Retain of an element of a collection made outside
  the loop (a list.index, through fields and directory chunks) that the
  function value takes (CallValue), which cannot borrow it and drops it
  after. An element that any use keeps (a call, a push, a struct, a
  return, on any path) is a real copy and not reported. Over compiler/xo
  the first version (any release in the loop) noted 17 loops that store
  the element on one path; this one notes none there.
- XO0909 (to do): a copy on write that can copy every iteration (a Reuse
  of a loop-carried list uniq cannot prove unique, naming the other
  holder).
- XO0910: a value over 8 words (compiler/arm64 layout) copied where the
  source shows no copy: a capture of a closure literal (anywhere: made
  once per call, each heap field shared), or a Variant (`Some(x)`) whose
  payload is an element read in a loop (a list.index in a loop block of
  a list that is a parameter or made outside the loop, not a map's
  directory chunk); the Variant itself may be in the loop's exit block
  (`return Some(x)` in a search). The first version noted any Option or
  enum value over 8 words: 11 notes over compiler/xo, most Options built
  in place (a constructor, a returned record) with nothing to copy from.
  The shipped rule gives 6 there (4 closure captures: ssa.Module 73
  words, Opts 88, ParsedIR 25, TestOpts 17; 2 values copied out of a map
  in a loop, test.xo) and 1 in the CRUD server (a 9-word UserPatch
  captured by the Mutex.with closure).

## Consequences

- `xo vet --cost` costs about an edit build (the front end and back-end
  caches apply); plain `xo vet` is unchanged.
- The notes describe what the compiler does today; a note can go away
  when the compiler improves, which is the point.
- Not in builds: cost notes appear only under vet (lead, 2026-10-21).

## Alternatives

- Checker-level heuristics (no SSA): cannot see which loop copies (the
  choice depends on the body's size, closures, defers, and the map's
  mode), so they would warn on every map loop.
- Notes in every build: noise in the agent loop; vet is where advisory
  findings go.
- A profile-guided report: needs a run and a workload; vet works on the
  source alone.
