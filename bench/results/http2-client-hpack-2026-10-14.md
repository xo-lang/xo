# The HTTP/2 client's HPACK in Xo (zero-C audit B11), 2026-10-14

Work item B11 of `zero-c-audit-refresh-2026-10-13.md`: the HPACK
encoder and decoder that xo_h2.c kept for the native HTTP/2 client (its
lines 87-669 in the audit's numbering, `hp_*`) leave the runtime; the
client's header blocks go through std/native/llrt/hpack.xo, which the
server has used since decision 0193 step 6.

## What moved

- **The hand off** (B10's pattern: C does the I/O, Xo the text). C never
  calls compiled Xo (decision 0180), so `rthttp.cl_start` now sets an
  HTTP/2 exchange up (a pooled connection, or a new one whose ALPN chose
  h2) and returns before its HEADERS with `cl_h2` true. Xo
  (std/native/llrt/http1.xo `h2_exchange`) takes the connection's HPACK
  state out of the C (`cl_h2_take`, an `rt.Any` the C keeps with the
  connection between requests, as the test world keeps MemFs state),
  applies the server's SETTINGS_HEADER_TABLE_SIZE values queued since
  the last request (`cl_h2_sizes`), lists the request's fields
  (httpcommon.EncodeHeaders: the pseudo fields, the header in lower case
  without the hop by hop fields, a cookie field per pair, content-length,
  accept-encoding gzip, user-agent Go-http-client/2.0) and encodes them
  with the new one field entry `hpack.Enc.field`, and sends the block
  (`cl_h2_send`). The C runs the exchange (`cl_h2_next`: the body within
  the windows, frames, flow control, errors) and stops at each complete
  response header block, which Xo decodes (`cl_h2_block`,
  `hpack.Dec.decode_part`, `cl_h2_fields`). `cl_h2_end` hands the state
  back, returns the connection to the pool or closes it, and sets the
  response or the failure; when the server did not take the request on
  a pooled connection (`cl_retry`) Xo calls `cl_start` again without the
  pool, as Transport retries it.
- **checkConnHeaders** (the `http2: invalid Connection request header`
  failures) moved to Xo with the field list.
- **Module layout**: http1.xo now imports hpack.xo (the client lives in
  http1.xo), so hpack.xo can no longer import http1.xo; the server's
  driver (`hpack.Conn`, which calls `http1.h2_target`) moved to http1.xo
  as `H2Conn`, and hpack.xo is HPACK only (`Enc.encode_job`,
  `Enc.set_sizes`, `Enc.field`, `TABLE`, the `Fields` fields pub).
- **Deleted from the runtime**: xo_h2.c's Huffman tables, static table,
  dynamic table, integer and string codecs, `hp_decode_part`,
  `hp_decode`, `hp_encode`, `hp_enc_setmax`, the client's
  `h2rq_emit`, `h2cc_exchange`, `h2_cl_pooled`, `h2_cl_fresh`.
  http_fuzz.c's client part stops at a header block (its decoding is
  Xo's), as its server part already did.

## Lines

git numstat of the port commit (9661e986):

| File | Deleted | Added |
|---|---|---|
| std/llrt/xo_h2.c | 769 | 219 |
| std/llrt/xo_http.c | 17 | 23 |
| **std/llrt, runtime** | **786** | **242** |
| std/native/llrt/http1.xo | 4 | 234 |
| std/native/llrt/hpack.xo | 73 | 47 |
| std/native/rthttp.xo, http.xo | 7 | 31 |

xo_h2.c went from 3,606 to 3,056 lines, xo_http.c from 2,543 to 2,549.
The runtime's C is 544 lines smaller net (the audit's estimate was 583
retired; the added C is the exchange's state and its entry points).

## The bar (decision 0180: within 5% of C's instructions)

Instructions retired (`/usr/bin/time -l`, the minimum of 5 runs) on the
Mac (Apple M4 Pro, darwin/arm64), through the heavy lock, other agents
running (counts, not wall clock). C: the xo of main c013b78d with its
std (the C HPACK); Xo: this branch at 9661e986. Ratio Xo / C.

`bench/perf/httpclient/measure.sh` with the new HTTP/2 workloads (N=3000
requests to a Go net/http server over TLS with ALPN h2, all on one
pooled connection; the server sends the nine header fields of the
HTTP/1.1 workloads; the client process is measured):

| Workload | arm64 C | arm64 Xo | ratio | LLVM C | LLVM Xo | ratio |
|---|---|---|---|---|---|---|
| h2len (256 B body) | 618,825,878 | 622,468,417 | 1.006 | 619,370,823 | 608,103,063 | 0.982 |
| h2big (64 KiB body) | 5,472,924,322 | 5,520,641,021 | 1.009 | 5,501,369,980 | 5,478,242,920 | 0.996 |

Every ratio is within the bar. The HPACK work per request is the same in
both workloads, so h2big's spread is the noise of TLS and frames over
64 KiB.

## Correctness

- A smoke run on the Mac (arm64, `XO_LEAKCHECK=1`, 20 requests each):
  h2len 264 and h2big 65,543 bytes of body and header fields a request
  (the C client's measurement run printed 196,629,000 for 3,000 h2big
  requests, the same 65,543), no live allocations at exit.
- linux/arm64 batch: see "Batch" below.
