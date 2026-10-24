# A var argument that is a List element copies it per call on the Go backend

Status: Observed 2026-10-23, fixed the same day (compiler/gen call.xo).

Symptom: on CI's Windows runner, every build by the checkout's xo.exe
spent 538 to 605 s in its link phase (run 40046075110), where the 0.27.0
archive's xo.exe built the same programs in 10 to 21 s. --phase-times
sub-phases (link.*, pe.*) put it in pe.link_pe: on the Mac, an xo built
with --backend=go took 85,735 ms to link a 6.6 MB windows/amd64 image,
a native xo 34 ms. The checkout's xo.exe was a Go backend build because
xobuild built xo that way on Windows (fixed the same day: amd64, as the
release does).

Cause: gen lowered a `var` argument that is a List element,
`put_le(bufs[k], at, v, n)` (one call per relocation in
compiler/macho link_pe), as a copy written back:
`tmp := bufs[k]; tmp.XoMark(); f(&tmp); bufs[k] = tmp`. The mark sets
the element block's sticky shared flag (list-own-marks.md), so the
callee's first write cloned the whole element: one 6 MB copy per
relocation. 20,000 calls on a 200K Int element took 2,101 ms; native,
a flat `var`, or `bufs[k][i] = x` directly took 0 ms. A var receiver
of a std method already went by address (place_addr); arguments and
user methods' receivers did not.

Fix: an element place (List indexes and fields, resolve_addr_place)
passed to a `var` parameter or receiver goes by address (place_addr:
the path's lists made unique, List_PtrM to the element) when no other
operand mutates its root. Decision 0111 rules out aliased `var`
arguments, so by reference and copy in and out give one result.
internal/gen TestVarElemInPlace times it (bound 2 s; the copying build
takes seconds). Still a marked copy: a `var` argument of a call
through a function value (call.xo, the `fx.vars` path) and a Map value
(not an element place).

How to avoid: on the Go backend, a copy of a List-holding value marks
it shared and turns the next write into a full copy. Code that
must stay linear passes an element by address or works on a local
(`var b = bufs[k]` moved out, then stored back once), not one marked
copy per call. When a build or run is slow, check how the xo itself
was built (`xo version --json` built_by, backend) before comparing
commits (agent-tools.md "Measuring").

Related: go-backend-list-set-quadratic.md, list-own-marks.md.
