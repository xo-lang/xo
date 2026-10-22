# 0327: `xo vet --cost`, notes on costs the source does not show

Status: Accepted (owner, 2026-10-21, from the agents' language poll; design
reviewed by the lead). Stage 1 implemented: XO0907. XO0908 to XO0910
claimed, to follow.

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
- XO0908 (to do): a dup and drop per iteration (a Retain of a
  collection element's leaf inside a loop, from insert_rc's leaves).
- XO0909 (to do): a copy on write that can copy every iteration (a Reuse
  of a loop-carried list uniq cannot prove unique, naming the other
  holder).
- XO0910 (to do): a value over 8 words held inline (an Option or record
  of an interface) or copied whole into a closure.

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
