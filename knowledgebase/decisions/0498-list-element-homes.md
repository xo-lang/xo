# 0498: A `var` argument that is a list element is passed at its slot on native

Status: Proposed (xo-lang-4, 2026-11-04). The native follow-up to
decision 0490, which the owner asked for when accepting it.

## Context

Decision 0151 passes a large `var` parameter (a struct or enum of
BY_REF_MIN = 256 bytes or more) by reference on the native backends,
and since 2026-10-21 a `var` argument that is a field of a value with
a home is passed at its address there (compiler/arm64/byref.xo, field
homes, `held.fslot`). 0490 brought that rule to wasm.

A `var` argument that is a list element was not covered. Decision 0169
reads it with `list.take` (the element moves out, its slot holds a
placeholder) and stores the new value back with `list.set` after the
call. On native the take loaded every word of the element, the call
copied them to a frame area and back, and the set stored them again.
tlsio.put's TLS 1.2 call `tls12.send(c.cl12[0], data)` was all 240 of
its 16 byte q register loads and stores (120 before, 120 after the
call), 2,384 bytes on darwin/arm64; its TLS 1.3 call on a field
(`tls13.send(c.cl, data)`) copies nothing.

LLVM copies around both calls (6,188 bytes, about 360 instructions
before each, no memcpy), so it needs both rules; that is not part of
this decision.

## Decision

On the native backends (arm64 and amd64, which share byref.xo), a
`var` argument passed by reference that is the element of a
`list.take`, whose new value's only use is the `list.set` storing it
back at the same index of the taken list (and that list's only use is
that store), is passed at the element's slot in the list:

- the take checks the index and computes the slot's address, but loads
  nothing and stores no placeholder;
- the call gets that address; the callee updates the element in place;
- the store back emits nothing.

The taken list is unique (0169: `reuse` comes before every take), so
no other value sees the update; nothing else reads the list between
the take and the store (its only use is the store), so no code sees
the slot mid-update. Such an argument gets no frame home
(find_take_args runs before plan_held).

## Consequences

- tlsio.put: 2,384 to 944 bytes; the fetch client's native binary
  5,993,554 to 5,977,043 bytes (-16,511). Measured 2026-11-04 on
  darwin/arm64 against the same base.
- A fault in the callee leaves the element as the callee left it, in
  the list, which the landing releases with its elements (the
  placeholder path released the list and the moved-out element
  separately). Golden list_elem_homes covers a fault mid-update under
  the leak check, a copy of the list kept before the update, a list in
  a field, and a local list.
- LLVM keeps copying; a later decision brings both 0490's field rule
  and this one to the LLVM emitter.

## Alternatives

- Keep the copy: 1.9 KB per call of a TLS 1.2 write path, for a callee
  of a few statements.
- A general escape analysis making every element reachable by address:
  larger, and the take and set pattern already proves what this needs.
