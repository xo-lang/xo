# 0327: `xo vet --cost`, notes on costs the source does not show

Status: Accepted (owner, 2026-10-21, from the agents' language poll; design
reviewed by the lead). Implemented: XO0907 (stage 1), XO0908, XO0909, XO0910.
XO0909 (Xo Lang 3).

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
- XO0909: a list update that copies the whole list every iteration: a
  Reuse (copy unless the count is 1) in a loop block whose list is a
  parameter of a loop header, the update's result passed back to that
  parameter, and a Retain of the same list in the loop's own blocks (the
  holder made in the iteration: `snaps.push(xs)`, a kept name). Not
  noted: a holder made before the loop (one copy, then the loop's list is
  unique), a loop updating on every iteration (rc.xo hoist_reuse moves
  the one copy to the entry), a list made fresh in the iteration, and a
  `var c = xs; c[i] = ...` copy (the language's meaning). A first version
  noting any Reuse left in a loop gave 14 notes over compiler/xo, all
  conditional pushes to a unique list (the count is 1 at run time); this
  one gives none there and none in the CRUD server.
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

## Addendum: XO0908 sees callback walks (2026-10-22, Xo Publish)

Measuring decision 0337 found XO0908 silent on the two common forms of
an element handed to a function value (gotchas/vet-cost-misses-callbacks.md),
whose cost was 3.3 times a `for` loop's instructions (5.5 times on a
list shared by two tasks):

- `for u in xs { t += f(u) }` over struct elements: insert_rc retains
  the element's heap fields (`field 1`, `field 2`) and the CallValue
  takes the element, so the test "a CallValue uses the retained value"
  never matched. elem_count_per_iter now follows the retained value's
  Field chain to the element and reads the uses of each; the note is at
  the call when the retained field has no position.
- `xs.any(fn(u) { ... })`: the loop is the prelude's (list_any), which
  cost_notes does not read. callback_walks notes the call in the root
  module's function: a call of the prelude's `map`, `filter`, `find`,
  `any`, `all` of a List, `filter`, `map`, `any`, `all` of a Map or Set,
  or a List's `fold`, when a parameter of its function value that takes
  an element (not fold's accumulator) holds heap memory. Not `sort_by`
  or `group_by`: their key runs once per element and the sort or the
  grouping costs more (+16% over 1000 structs of two strings).

Over compiler/xo: 7 notes (`any` and `map` over strings and tuples,
none in a hot path; 18 when sort_by was included); none in the CRUD
server.
TestVetCostCallbacks. Decision 0337 (Proposed) would remove the cost
itself; until then the note names it.
