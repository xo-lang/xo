# 0490: A `var` argument that is a field of a home is passed by its address on wasm

Status: Proposed (xo-lang-4, 2026-11-04). Prototype on branch l4-fhome; the code lands only after the owner accepts.

## Context

Decision 0179 (stage 3, compiler/wasm/home.xo) passes a large `var`
parameter (a struct or tuple of HOME_MIN = 16 leaves or more, of a
function only called directly) as the address of memory holding it, its
home, instead of its leaves. The caller passes the home of a chain value
it gives up, and otherwise a temporary block holding a copy, which it
reads back after the call a word at a time.

A `var` argument that is a field of a value the home holds always took the
second path. `tls13.take(c.cl)`, with `c` a `var` Conn (a home) and `cl`
its 968 byte TLS 1.3 client, allocated a 968 byte block, copied the
client into it, called, and stored each of the client's words back into
the home: tlsio.take_out, take_app and put were 18 KB each of an HTTPS
client component (6,700 WAT lines each, about 1,260 stores), for callees
whose bodies are two statements. A read only parameter already passed a
field's address (mem_of); only the `var` one copied.

Native backends make the same copy, more compactly: on darwin/arm64,
tlsio.put is 2,384 bytes, 240 of its instructions 16 byte q register
loads and stores, about 1.9 KB copied per call. LLVM's (darwin/arm64,
the same fetch client) is 6,188 bytes, 578 of its instructions on q
registers: the client copied in and back out. Measured 2026-11-04 with xo
at eab4586dd.

## Decision

On wasm (plan_field_calls in home.xo, after plan_nested), a call v of a
function taking parameter pi by reference passes the address of the field
in the home, with no temporary and no read back, when:

- the argument a is `field i` of a chain value c the home holds, used
  only by v;
- the call's `var` result for pi has one read r, whose one use is u =
  `setfield i of c to r`, which the home then holds;
- nothing between v and u (u in v's block or in a block whose one
  predecessor is v's) uses a value the home holds or a view of it, but r;
- c's other uses cannot see the callee's writes to field i: each reads
  another field, or runs before v and not after it through a loop.

r is then a view of the field in the home (mfield, moff), and u stores
nothing (inhome, as plan_nested's nested updates). A call that can
suspend passes the same address after a resume (no pre-copy). A fault
exit zeroes the homes as before.

Native arm64, amd64 and LLVM are not changed by this decision. They make
the same copy (Context), smaller in code but the same work at run time; a
follow-up decision ports the rule to their `var` passing once a native
--ab on a TLS row shows what it saves.

## Consequences

- Sizes (wasip2 components built on the Mac, xo at 07066b4a5 against the
  prototype rebased on it): hello 315,576 and echo 77,255 bytes,
  unchanged (no `var` call on a field of a home); the HTTPS client
  (xo-fetch) 2,071,821 to 2,065,807 (-6,014). Before 07066b4a5 (tlsio's
  take_out and take_app rewritten to take in place, the std side of this
  problem) the same prototype saved 11,288 (2,081,099 to 2,069,811). Its three tlsio functions are still about 5,700
  WAT lines each: the rest is the TLS 1.2 path's whole-element read and
  write of `c.cl12[0]` (Alternatives, element homes).
- The probe golden field_homes (an Outer holding a 22 leaf Inner, taken
  and updated through `var` calls): 19,240 to 17,878 bytes; take_out 323
  to 56 WAT lines, put_in 307 to 40; output equal; 6 allocations instead
  of 9, none live at exit.
- Run time, wasm (linux/arm64 host 1, wasmtime 50, 7 alternating runs
  each, wall time with start-up): a program of 2,000,000 `var` calls on a
  field of a home (take and put through an Outer holding a 22 leaf Inner,
  as tlsio's Conn and client; ~/xo-lead/notes/xo-lang-4/out/fhb.tgz):
  median 103 ms at 07066b4a5, 57 ms with the prototype (0.55x the time),
  output equal; the module 13,651 to 12,289 bytes. The http rows of
  bench/results/wasip3-stream-client-2026-11-04.md (echo, proxy) do not
  change: the hello and echo components are byte-identical with and
  without the prototype. There is no wasm TLS timing row; the fetch
  client's three tlsio calls are the case the program above measures.
- The prototype (branch l4-fhome) passed the wasm command goldens
  (wasip1, wasip2), the components, the arm64 and LLVM goldens and
  -short on host 1. The fixed point is not affected: compiler/wasm is
  outside the self build.

## Alternatives

- In std only: tlsio does the take in place (l4-take, 9,335 bytes of the
  fetch client). It fixes one function; every other `var` call on a field
  of a home keeps the copy.
- A list element read only through field reads loaded where read
  (element homes): the other half of tlsio's size (`c.cl12[0].out` reads
  and writes the whole 1,136 byte element); a separate step, after this.
