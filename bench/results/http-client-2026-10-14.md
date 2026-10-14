# The HTTP/1.1 client and URL handling in Xo (zero-C audit B10), 2026-10-14

Work item B10 of `zero-c-audit-refresh-2026-10-13.md` (row 5 of its
order, two items): the native client's response reading and the URL
parsing still in C move to Xo, and the C request parser they kept
alive leaves the runtime.

## What moved

- **Item 1, the client** (std/native/llrt/http1.xo): `client_do` is
  xo_cl_do in Xo (the method check, Header.Set canonical names, the
  URL through `url_parse`, the redirects of net/http's Client:
  redirectBehavior, defaultCheckRedirect's 10, the headers a redirect
  drops, refererForURL, the Location resolved against the URL, Net.only
  on each redirect through netpolicy.xo), and `parse_response` with
  `read_response` is cl_read_response in Xo (the status line and header
  up to the first empty line, informational responses but 101 skipped,
  Content-Length, chunked bodies through http1.xo's `chunked`, a body
  to the end of the stream, HEAD, 204, 304). The C keeps the connection,
  TLS, HTTP/2, and the request's writing: `xo_cl_start` dials, does the
  TLS handshake, sends the request (or runs the whole HTTP/2 exchange,
  whose HPACK is B11), and returns; Xo reads the response through
  `xo_cl_input` (up to the end of the first head, the same `head_len` as
  the server's `xo_hc_input`), `xo_cl_fill`, `xo_cl_take`, and
  `xo_cl_skip`, then `xo_cl_end` closes the connection and makes the
  error text with a prefix Xo passed (`Get "url"`). C never calls
  compiled Xo (decision 0180), so this is the server's pause and resume
  shape (decision 0193 step 6) with the client's loop in Xo.
- **Item 2, the URLs**: the C URL parser (net/url ParseRequestURI and
  Parse, the host, userinfo, and port checks, unescaping, ParseQuery)
  had two callers left: xo_cl_do (now Xo) and the HTTP/2 server's
  h2_request (xo_h2.c). For the second, hpack.xo passes the parse of the
  first :path of each decoded fragment (`http1.h2_target`: the path,
  whether it parsed, scheme, host, path, raw query, the query) with
  `rthttp.h2_fields`; xo_h2.c keeps the block's first one and h2_request
  takes it when it is the request's :path (a :path it has no parse for
  is malformed, a stream error as before). The HTTP/2 client takes the
  request target Xo escaped (`http1.escape_path`) instead of a C URL.
- **Deleted from the runtime**: xo_http.c's URL parser and query parser
  (h_ishex, h_unhex, h_host_ok, h_unescape, h_parse_host,
  h_valid_userinfo, h_url_parse, h_parse_target, h_parse_query), the
  request head parser hp_head with h_line and h_vbyte, the chunked
  decoder hb_chunked, h_escape_path, h_split_hostport, url_op,
  cl_read_response, cl_once, valid_method, xo_cl_do, and the
  HTTP_MAX_HEAD and HTTP_MAX_FIELDS limits; xo_rt.c's `net_admits` and
  cabi.xo's `net_admits` export (its last C caller was xo_cl_do).
- **Moved to test data**: hp_head, hb_chunked, and the URL parser under
  them are `std/llrt/testdata/http_ref.c` (996 lines), which only
  http_diff.c and http_fuzz.c include (they play http1.xo's part with
  it, as decision 0198 set up); no runtime build compiles it, and it
  goes with those tests (as crypto_ref.c does). http_fuzz.c's client
  part (cl_read_response) is gone: the Xo reader is tested by
  TestHTTP1ClientDiff with mutated responses.
- **Kept, and why**: h_tchar, h_lower, h_canon, h_server_token,
  h_has_token (the response writer, B12, and xo_h2.c call them);
  h_valid_port, hurl, hu_free, hp_cmp (hc_port, the request struct,
  hw_fields); h_go_quote (the HTTP/2 client's error texts, until that
  client moves); h2_request itself (its checks build the C request the
  C response code reads; only its URL parse moved).

## Lines

git numstat of 16524ee6..3e37c4b7 (the port, before the merge of main):

| File | Deleted | Added |
|---|---|---|
| std/llrt/xo_http.c | 1,595 | 213 |
| std/llrt/xo_h2.c | 29 | 52 |
| std/llrt/xo_rt.c | 5 | 0 |
| **std/llrt, runtime** | **1,629** | **265** |
| std/native/llrt/http1.xo | 6 | 557 |
| std/native/llrt/cabi.xo | 9 | 0 |
| std/native/rthttp.xo, http.xo, hpack.xo, prelude.xo | 23 | 49 |
| std/llrt/testdata/http_ref.c (test data, not compiled into programs) | 0 | 996 |

xo_http.c went from 3,925 to 2,543 lines, xo_h2.c from 3,583 to 3,606
(the stored :path parse and its clearing). The runtime's C is 1,364
lines smaller net; the audit's B10 estimate was 1,558 retired (it did
not count xo_cl_do, which moved too, and counted the helpers the writer
and HTTP/2 still call).

## The bar (decision 0180: within 5% of C's instructions)

Instructions retired (`/usr/bin/time -l`, the minimum of 5 runs) on the
Mac (Apple M4 Pro, darwin/arm64), through the heavy lock, other agents
running (counts, not wall clock). C: the xo of main 16524ee6 with its
std (the C client); Xo: this branch at 3e37c4b7. Ratio Xo / C.

Client (`bench/perf/httpclient/measure.sh`, new: N=3000 requests, each
on a connection of its own, to a Go server sending fixed responses
with nine header fields; the client process is measured):

| Workload | arm64 C | arm64 Xo | ratio | LLVM C | LLVM Xo | ratio |
|---|---|---|---|---|---|---|
| none (start and end) | 28,144,540 | 28,096,659 | 0.998 | 26,093,297 | 25,959,600 | 0.995 |
| len (256 B body, Content-Length) | 856,077,893 | 852,362,426 | 0.996 | 845,818,850 | 835,613,753 | 0.988 |
| chunked (4 KiB in 32 chunks) | 899,490,092 | 900,688,721 | 1.001 | 891,404,768 | 875,460,766 | 0.982 |
| big (64 KiB, Content-Length) | 1,062,344,989 | 1,015,968,595 | 0.956 | 1,050,126,854 | 1,000,257,055 | 0.953 |
| redirect (302, then len) | 1,619,765,386 | 1,613,133,738 | 0.996 | 1,607,619,144 | 1,585,349,403 | 0.986 |

HTTP/2 server with the :path parse from Xo (`bench/perf/httpparse`,
N=20000; the server process is measured), and HTTP/1.1 for reference:

| Workload | arm64 C | arm64 Xo | ratio | LLVM C | LLVM Xo | ratio |
|---|---|---|---|---|---|---|
| h2_get | 5,121,241,071 | 5,164,128,004 | 1.008 | 4,884,143,967 | 4,912,424,187 | 1.006 |
| h2_varied | 5,288,170,230 | 5,324,853,243 | 1.007 | 5,024,667,775 | 5,056,391,769 | 1.006 |
| h1_get | 2,259,321,123 | 2,292,049,396 | 1.014 | 2,067,128,365 | 2,079,113,389 | 1.006 |
| h1_url | 3,004,734,262 | 3,040,469,905 | 1.012 | 2,878,523,710 | 2,896,326,701 | 1.006 |
| h1_chunked | 1,357,645,760 | 1,288,250,253 | 0.949 | 1,207,052,248 | 1,131,877,038 | 0.938 |

Every ratio is within the bar. h1_get and h1_url run code this item did
not change (the server's head parse was Xo before; `xo_hc_input` only
calls the shared `head_len`), so their 1.006 to 1.014 is the noise of
the machine. h1_chunked compares Xo with Xo: the first measurement of
the client's chunked workload was 1.123 (arm64) and 1.104 (LLVM),
because http1.xo `chunked` looked for `;` and `\r` in a chunk size line
with `rt.str_find`, which searches on to the end of the input, so every
chunk scanned the rest of the body; a bounded byte scan (`bfind`) fixed
it for the client and made the server's chunked requests 5 to 6
percent cheaper (gotchas/str-find-searches-to-the-end.md).

## Correctness

- **TestHTTP1ClientDiff** (new, internal/llvm/http1_client_diff_test.go):
  a client program built with LLVM and arm64 requests 457 responses (19
  that the native client and net/http read alike, 38 edge cases where
  the native client is lenient or differs, 400 random of which every
  second is mutated) from a server that sends them raw, redirects
  included; each output line must equal testdata/http1_client.txt,
  which the C client wrote (the run with `XO_CLI` = the xo of 16524ee6
  and `-golden-update`); the Go backend (net/http) must print the same
  for the 19. The Xo client printed all 457 lines byte for byte as the
  C did, on both backends (Mac, 2026-10-14). One response first counted
  as agreeing (a chunk size `zz`) is where they differ: net/http says
  `invalid byte in chunk length` without the request, the native
  client `Get "url": malformed chunked encoding`; it moved to the edge
  cases.
- Deliberate differences from the C, none in the corpus: a URL or a
  Location with a NUL byte now fails to parse (the C cut it at the NUL,
  a C string); error texts are no longer cut at 511 bytes (the C's
  buffer).
- The http goldens (http_client, http_fault, http_server, http_stream,
  http_tls, http_wire, net_only) pass on arm64 on the Mac; the batch
  on linux/arm64 (EC2) runs vet, `-short`, the goldens with arm64 and
  LLVM, TestHTTP1ClientDiff, TestHTTP1ParseDiff, TestHTTPParseDiff,
  TestHTTPFuzz, TestH2Diff, TestH2Sanitizers, TestGoldenASan,
  TestTLSDiff, and the fixed point (see "Batch" below).
- internal/arm64's asm goldens changed only by the removed
  cabi.net_admits (every program carries cabi).

## Batch

PENDING
