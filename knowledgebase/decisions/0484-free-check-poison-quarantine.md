# 0484: XO_FREE_CHECK, a native debug mode that poisons freed blocks and retired records

Status: Proposed (Xo Lang 1, 2026-11-03; the Lead asked for it)

## Context

On 2026-11-03 golden http_reverse_proxy_ws trapped 5 of 5 as a wasip2
command after ab852cf42 and passed 5 of 5 on native, XO_RC_CHECK=1
included (gotchas/native-silent-on-freed-llrt-records.md). The proxy's
tunnel task kept writing on a client connection that the stream's end
had closed: close_at freed the connection's object and buffer and
zeroed its fields in the response record, which lives on. Native read
descriptor 0 (standard input) and its send failed quietly; wasm read
handle words from address 0, which is memory there, and wasmtime
rejected the handle. The same day's race stress found seven std use
after frees (decision 0460). Native had no mode that turns such a use
into a fault: XO_RC_CHECK checks decision 0322's uniqueness proofs,
XO_LEAKCHECK counts blocks never freed, XO0915 follows a raw address
inside one SSA block, and TSan sees only two threads touching memory
at once.

## Decision

XO_FREE_CHECK=1, read once at the program's start (entry.xo start),
off by default, native only (a sanitizer or --race build, which has no
page blocks, ignores it):

1. Every free is held. The program runs on per thread heaps from its
   start (the switch cabi.xo heap_threads makes before a second thread
   runs), counting stays on (alloc.xo COUNT), and every record's
   magazine sizes are far below any list's length (MAG_CAP), so every
   allocation takes alloc.alloc and every free alloc.free_mt; free_mt
   hands the block to heapos.free_hold through rt.heap_grow(h, FHOLD),
   as the heap profile and the scavenger do. The single threaded and
   inlined paths (alloc_fast, free, free_push, resize) are unchanged, so
   the mode costs nothing when off.
2. A held block of a size class is filled with POISON
   (0xdeadbeefdeadbeef: outside every user address space, so a load
   through it faults; huge as a count or length) and takes the next slot
   of its thread's ring of 65,536 blocks. The block it replaces is
   checked still all POISON, else the program ends with "fault:
   XO_FREE_CHECK: a freed block was written after its free" (status
   70), and goes back to its list (alloc.free_held). At the program's
   end every ring is checked the same way (entry.xo finish). Page blocks
   and big blocks are freed at once.
3. A record part that is closed while the record lives on is retired
   with heapos.retire: zeros normally, POISON in the mode. hclient.xo
   close_at retires a client connection's fields, so a send or read on
   a closed connection faults at its first load through them.

The proof: at ab852cf42 with this change, http_reverse_proxy_ws on
native darwin/arm64 crashes 3 of 3 under XO_FREE_CHECK=1 with
"SIGSEGV ... at address 0xdeadbeefdeadbeef" and the trace
tlsio.write < hclient.send_chunk < http.tunnel (http.xo:4802), the
use the wasm trap pointed at; without the mode it passes. At 1af909c53
(the lifetime fix) it passes with the mode on.

## Consequences

- A use after free of a heap block or a retired record faults where it
  happens on native, with the crash report's trace; a write after free
  is reported when the block leaves the ring or at the end.
- The mode is slower (a poison fill per free) and holds up to 65,536
  freed blocks per thread; freed blocks never move between threads.
- A block is checked only while held: a use more than 65,536 frees of
  its thread later is missed, as with any quarantine.
- Other records with closed parts can opt in with heapos.retire.
- Survey (2026-11-03, the second linux/arm64 host, at 1af909c53 plus
  this change, xo itself running under the mode too): the 83 network
  goldens (http, net, tcp, udp, tls, ws, conn, proxy, h2, dial,
  listen, websocket, sock, smtp, dns, grpc, serve) on arm64 all pass
  (98 s); internal/stdtest passes but for TestArchiveFuzzCorpusCurrent
  and TestJsonFuzz, which fail the same way without the mode (their
  generated corpora differ from what that host's Go writes). No new use
  after free: the day's seven were fixed first (decision 0460).

## Alternatives

- A generation word per object, checked at every use: catches more but
  changes every object's layout and every load the compiler emits.
- Poison without a quarantine: the next allocation of the class reuses
  the block at once, so a use soon after the free reads a live object.
- Zero instead of POISON for retired fields: what close_at did; zero is
  a valid descriptor and, on wasm, a valid address.
- Extend XO_HEAP_GUARD (libcmem.xo guard_large): a build time mode
  that ends each of the runtime's large records against an inaccessible
  page, so an overrun faults; it is per build (a codegen input) and
  about overruns of rt.heap_large records, where this is per run and
  about every heap block's and a closed record's lifetime. The two
  combine (the goldens run with XO_HEAP_GUARD=1).
- Never reuse freed blocks: simplest, but memory grows without bound in
  a long test.
