# 0460: Keep a value alive to the end of the statement that takes its raw address

Status: Proposed (Xo Lang 1, 2026-11-02; the owner decides; drafted at the Lead's request)

## Context

A value is released right after its last use, even inside the arguments of
the call that uses its address (gotchas/raw-address-outlives-bytes.md):
`f(raw.data(s), s.len())` releases an owned `s` before `f` runs, so `f`
reads freed memory. std works around it by hand with `raw.alive(b)` /
`raw.alive_str(s)` after the last raw access. The 2026-10-16 audit fixed 15
such sites; the gotcha was observed again 2026-10-19 (tlssrv.xo), and the
2026-11-02 --race stress found more in shipped std (h2w.xo content-length,
dns.xo config and hosts text, and the reports still being triaged). The
non-race build reads correctly only while the freed block is not reused, so
the bugs are silent until an allocation lands in between (another thread,
or a later change that allocates in the callee).

The 2026-11-02 numbers. The race stress (every concurrent golden built with
`--race` on linux/arm64, where the heap is glibc's and ThreadSanitizer sees
each free) found five such bugs in shipped std, each fixed with a
`raw.alive*` and a row in internal/llvm TestRaceStdNoUseAfterFree:
h2w.xo's HTTP/2 content-length (since v0.12.0, 45a6139ed), http1.xo's
chunked request body (v0.12.0, 946133ed6), websocket.xo's mask key
(v0.34.0, 281727970), dns.xo's resolver configuration and hosts text
(v0.35.0, 1fb036fe2), and raw.u8s_copy_back's grown inflater output
(v0.34.0). A lint, `xo vet --cost` XO0915 (an address used after its value
is released in the same SSA block), was then written: run on the std from
before the fixes it found 1 of the first 4 (h2w; it missed dns, websocket
and http1, whose address crosses a block or a closure), and it found 8
more sites in current std, of which 5 free a fresh value (real), 2 release
a shared one (a decrement, benign until the sharing changes) and 1 is an
immortal list (false). A lint that follows addresses across blocks and
closures is a dataflow analysis of its own; the compiler already knows
both the address and the release.

## Decision (proposed)

The compiler extends the lifetime of a local to the end of the enclosing
statement when the statement takes its raw address: an argument
`raw.data(x)`, `raw.bytes_addr(x)`, `rt.obj_addr(x)` (or any function
marked as returning an address into its argument) delays x's release to
after the statement, as if `raw.alive(x)` followed it. Addresses kept past
the statement still need an explicit `raw.alive` (or a field that holds the
value), as today.

## Consequences

- Removes the in-statement class without a lint or hand-written keep-alives;
  existing `raw.alive*` calls stay valid (a later use) and can be removed
  over time.
- One release moves later by at most one statement: no change in peak
  memory beyond the statement's own temporaries.
- Touches the drop placement in compiler/ (the last-use analysis), so it
  needs the fixed point and an identity check limited to the affected
  functions; the marking of address-returning functions is a new attribute
  or a fixed list (raw.data, raw.bytes_addr, rt.obj_addr).

## Alternatives

- `xo vet` lint (raw address of a local whose last use is in the same
  statement): catches the class, leaves the fix to the author; planned
  either way as a check for addresses kept past the statement.
- Keep the hand-written `raw.alive*`: the status quo; the class recurs.
- Make raw address functions borrow until the end of the enclosing
  function: simpler rule, holds memory longer (big Bytes in loops).
