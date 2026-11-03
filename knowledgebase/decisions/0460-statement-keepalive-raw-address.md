# 0460: Keep a value alive to the end of the statement that takes its raw address

Status: Accepted (owner, 2026-11-02, on the count below; drafted by Xo Lang 1 at the Lead's request)

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

## Decision

The compiler extends the lifetime of a local to the end of the enclosing
statement when the statement takes its raw address: an argument
`raw.data(x)`, `raw.bytes_addr(x)`, `rt.obj_addr(x)` (or any function
marked as returning an address into its argument) delays x's release to
after the statement, as if `raw.alive(x)` followed it. Addresses kept past
the statement still need an explicit `raw.alive` (or a field that holds the
value), as today.

## Consequences

- The owner's condition, counted 2026-11-02: of the 7 std bugs fixed that
  day (5 found by the race stress, 2 pushes from the lint), 6 had the
  address and the release in one statement and would have been prevented:
  h2w.xo's content-length, http1.xo's chunked body, websocket.xo's mask key
  (the value is a temporary, `raw.bytes_addr(llrand.next_bytes(4))`, so the
  rule covers temporaries as well as named locals), dns.xo's configuration
  and hosts text (3 sites), the runtime's fault and error messages (8 sites
  in cabi.xo and crash.xo) and h2srv.xo's trailers and Content-Length (2
  sites); by site, 16 of 17. All 5 real XO0915 hits are among them. The one
  not prevented is raw.u8s_copy_back, which keeps the address in a local
  (`var a = rt.obj_addr(out)`) and uses it statements later; that shape
  still needs `raw.alive` (or a field holding the value), and the race
  build stays its check.
- As implemented (Implementation below), the pass follows an address
  across the blocks of its function, so it also prevents the seventh bug:
  with every hand keep-alive removed, all 7 of the day's bugs (17 of 17
  sites) are covered, where the statement rule counted above covered 6
  (16 of 17). The race test proves it on the five goldens (2026-11-02).

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

## Implementation (2026-11-02, Xo Lang 1)

compiler/ssa rc.xo `keep_addr_values`, before the liveness pass of
insert_rc: for each address taken from a value (`raw.data`,
`raw.bytes_addr`, `rt.obj_addr`, or an add or sub of one), a borrowing
marker call of the value (`rt.addr_keep`) goes right after the address's
last use in every block that uses it, so the value's release is placed after
each use; `drop_addr_keeps` removes the markers once the releases are
placed. The uses are followed across blocks, which the first version (per
block) missed: at this pass a statement's arguments are still split into
several blocks (the landing checks of the calls among them), joined only
after it. A use followed by its landing check (`rt.landing` or
`rt.unwinding`) gets its marker first in the check's continuing block,
never between the operation and its check (the inlined module's verifier
failed on that). An address passed to a block parameter keeps its value
only to the jump. That reaches further than the decision asked: it also
keeps raw.u8s_copy_back's list alive to the copy in a later block, the case
the count above says a statement rule would not prevent.

Proof (2026-11-02, linux/arm64, host 2): with the change and the
keep-alives of the five race fixes removed (h2w, http1, websocket, dns,
raw.u8s_copy_back), internal/llvm TestRaceStdNoUseAfterFree passes on all
five goldens; without the change each fails with a use after free. The
hand keep-alives stay in std (they are harmless and keep older compilers
right). `XO_PASSES=-addr_keep` turns the pass off, to bisect a use after
free to it; `-addr_keep_drop` leaves the markers in a dump. The rc cache
version is v12, and its key holds both switches. XO0915 stays as a guard
for an address the pass does not follow (one carried in a field or through
a block parameter).
