# Chaining landing pads lost a memory home on the hot path

Status: Observed, fixed (2026-10-18).

## Symptom

chain_pads (compiler/ssa chainpads.xo) shrank every function, yet
httpparse h2_get ran 1.6% more instructions. Callgrind of the HTTP
server's h2 scenario showed +4.6% a request, nearly all of it in
tlssrv.recv_some, feed, and send. recv_some's fast path, which had read
`c.s12` in place and called tls13srv with the pointer, now copied dozens
of words of `var c: Conn` onto the stack at entry.

## Cause

compiler/arm64 byref.xo tracks which value a memory home (a large `var`
parameter passed by reference, or a frame home) holds at each
instruction. A pad after a call that writes the home starts with its
contents unknown. Chaining pad P into pad Q gave Q a second predecessor.
When P came after such a call, the meet at Q was unknown, so Q's reads of
the value failed the "held at every use" test. The value was then loaded
whole where it was made, on the normal path.

## Fix

A pad that reads a value of a type that may have a home is never a chain
target: a struct or enum of 256 bytes or more (BY_REF_MIN) that the
function takes as a `var` parameter or passes to a callee's
(chainpads.xo home_types). The xo self build saves 2.96 MB instead of
4.11 MB, and the h2 numbers are back within noise. An SSA pass that adds
edges into cold blocks must check what the native backend's dataflows
meet there: measure with the Mac sweep, not only the code size.
