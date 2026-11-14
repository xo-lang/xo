kind: review
module: http
part: the HTTP/1.1 server and client (std/native/llrt/http1.xo, http1w.xo, hclient.xo), the HTTP/2 server and client (h2srv.xo, h2w.xo, hpack.xo, http1.xo h2_*), std/native/http.xo (router, file server, serve_content, ReverseProxy, CrossOriginProtection), std/url.xo and neturl.xo, std/multipart.xo, std/textproto.xo, std/cookiejar.xo, std/httpproxy.xo, std/idna.xo, the prelude Fs, and the Go backend's std/rt/http_net.go and caps.go
result: pass
commit: 849937377
source: fb593fbdd15c360007868adda6d8fa4b0028b58c34dd8acf13faf3594ec09f86
date: 2026-11-13
host: darwin/arm64 and linux/arm64 hosts 1 and 2 (probes and tests against go1.26.1, then go1.26.11 after the toolchain bump)
command: read against knowledgebase/infrastructure/std-security-checklist.md; Go vulnerability database sweep (net/http, net/http/internal, net/http/internal/http2, net/http/httputil, net/http/cgi, net/http/fcgi, net/http/cookiejar, net/url, net/textproto, mime/multipart; with golang.org/x/net/http2, http/httpguts, http/httpproxy, idna where they share an entry)
measure: 56 Go advisories checked: 3 not applicable, 36 not shared, 17 affected Xo (13 shared, 4 unclear and then fixed) and fixed with a test that fails without the fix; 3 more defects outside Go's list found by the first reader (HTTP/1 head parse and textproto read_header quadratic, HTTP/2 buffered bodies unbounded per connection) and 10 by the second (section 6), all fixed; 1 intended difference from Go 1.26.11 kept (section 5)

Security review of std/http for decision 0616, by aws-43. First reader:
aws-43, reading every entry against the code at f557f6e3d and naming
the hostile input and the answer Xo should give. Second reader: Xo Lang
2, who read the fixes as a stack and whose findings are section 6.
Every Shared or Unclear verdict became a fix with a test that fails
without it (a golden, a module test or a diff test against net/http);
each fix has an entry in std/changes (http.md, url.md, multipart.md,
textproto.md, prelude.md) and a Security line in CHANGELOG.md's
Unreleased section. Affected releases: every release up to 0.50.1, on
the backends each entry names.

The Go backend uses its toolchain's net/http. On go1.26.1 it lacked the
GO-2026-66xx fixes; the toolchain moved to go1.26.11 on 2026-11-13
(owner item 9, Lang 2's eb10d0186), and where Go 1.26.1 differed from
what Xo promises on every backend, std/rt closes the gap itself (the
CONNECT client in 33b732005, HTTP/2 response fields in b340e6ebd,
Windows paths in 1759348e9).

## 1. Go's CVE history

Shared and Unclear entries, each now fixed:

| Advisory | Defect | Xo before | Fix and test |
|---|---|---|---|
| GO-2023-1878 (CVE-2023-29406) | client sends a Host or User-Agent holding CR or LF | with_host and User-Agent written unchecked (hclient.xo) | b783c5b12: an invalid Host fails before connecting, newlines in other fields become spaces; golden http_request_guards |
| GO-2023-1571 (CVE-2022-41723) | HPACK re-decodes a field cut across CONTINUATION frames | the Huffman name decoded again per fragment | 27438f116: a field is decoded once, when both strings are whole; golden and h2 diff cases |
| GO-2026-6611 | SETTINGS INITIAL_WINDOW_SIZE walks every stream per frame | a walk per frame | ac687c16d: one store, the streams read the delta; SETTINGS and WINDOW_UPDATE work bounded |
| GO-2026-6603 | declared Trailer names unbounded | no cap | 66548f114: kept once each, at most 64 |
| GO-2026-6610 | HTTP/2 client accepts malformed framing fields | connection specific fields kept, Content-Length loosely parsed | c304f6c53 (native), 7005ae3d0 (Content-Length up to 2^63 - 1), b340e6ebd (Go backend on go1.26.1); TestH2Diff |
| GO-2026-6613 | no close after a 2xx answer to CONNECT | the connection read on | eb7a393c2, bb237a6d7 (closes after any CONNECT answer; a 2xx one has no framing fields); TestHTTP1ParseDiff, golden http_request_guards |
| GO-2026-6605 (Unclear) | CONNECT requests sent framed and pooled | client pooled the connection, ReverseProxy forwarded CONNECT | bb237a6d7, 33b732005 (never pooled, streamed open too; ReverseProxy answers 405); golden http_client_connect_pool |
| GO-2022-1038, GO-2026-4976 | ReverseProxy forwards a raw query the hook did not see | raw query forwarded | 3c079900e: only the query parse and the rewrite hook saw, native and Go backends; golden http_request_guards |
| GO-2026-4341 | parse_query splits before counting | split first | 00db248c0 (url.parse_query), 26bf45db0 (Request.query_all, all backends): counted first, 10000 at most; golden http_form |
| GO-2026-6608 | a negative multipart header limit means none | unbounded | 40277cb5d: never below zero |
| GO-2024-2599 (CVE-2023-45290) | streamed multipart line unbounded, copied per refill | no cap, quadratic | 6c719ddc5: stops at the header limit while read, gathered once |
| GO-2026-5039 | protocol errors carry raw input | raw bytes in error texts | ea70c82ee (Lang 2): quoted as Go 1.26.2 and later |
| GO-2022-1143 | Windows device names reached through Fs | no check | bada05b41 (native), 1759348e9 (Go backend: filepath.IsLocal); golden http_file_server_devices, run on the windows/arm64 VM (403) |
| GO-2021-0224 (Unclear) | 100 Continue after the response head | could follow a written head | 4c6c55a27, 968197a15 (the state changes atomically; a head waits out a 100 being sent); golden http_continue_after_head |
| GO-2022-0969 (Unclear) | linger not enforced while the peer sends | 1 s linger only while silent | 4b1eec4fe: closed at the deadline also while input arrives |
| GO-2026-6609 (Unclear) | Range count unbounded | no cap | 3b04ae086: more than 200 ranges ignored, as Go 1.26.11 |

Not shared (the feature exists and the code avoids the defect; read at
f557f6e3d, and covered by TestHTTP1ParseDiff's 2583 inputs,
TestHTTPParseDiff's 3081, TestHTTP1ClientDiff, TestH2Diff and the
TestHTTPFuzz harnesses against net/http where the entry is about
parsing): GO-2021-0159 (CL with TE, repeated CL, "Content Length"),
GO-2022-0212 (space before the colon), GO-2022-0525 (invalid
Transfer-Encoding as chunked), GO-2023-2382 (chunk extension bytes),
GO-2025-3563 (bare LF in a chunk line), GO-2022-0236 (7 MB header
panic), GO-2023-1704 (preallocation by newline count), GO-2025-4015
(continuation lines quadratic), GO-2025-4012 (cookie count),
GO-2021-0172, GO-2023-1569 and GO-2023-1705 (ReadForm files and memory),
GO-2022-0536 (PING and RST_STREAM floods), GO-2023-2102 (rapid reset),
GO-2024-2687 (CONTINUATION flood), GO-2022-0288 and its follow-up
(canonical key cache), GO-2026-6089 (h2c preface sniff timeout),
GO-2026-4918 (MAX_FRAME_SIZE 0 loop), GO-2026-6612 (double refund),
GO-2026-6617 (HPACK table race), GO-2024-2600, GO-2025-3420 and
GO-2025-3751 (credentials across redirects), GO-2024-2963 (100-continue
client state), GO-2025-3503 (zone against NO_PROXY), GO-2022-0761
(Proxy header under CGI), GO-2026-5026 (Punycode to ASCII), GO-2022-0211,
GO-2025-4010 and GO-2026-4601 (URL host forms), GO-2021-0241, GO-2021-0245
and GO-2022-0520 (ReverseProxy hop-by-hop, panic, X-Forwarded-For),
GO-2026-6859 (MaxBytesReader race), GO-2025-3955 (CrossOriginProtection
bypass).

Not applicable: GO-2021-0226 (CGI and FastCGI content type; Xo has no
CGI host, and its own server sniffs the type), GO-2022-0988 and
GO-2026-6218 (URL features Xo does not have).

## 2. Limits on hostile input

Every limit below is Go's unless the row says otherwise.

- Request head: 1 MiB + 4096 (with_max_header_bytes + 4096), 431;
  10000 fields at most (Go counts bytes only). Read once in linear
  time while it arrives (308296618: a 1 MiB head sent a byte at a time
  was quadratic).
- Chunked bodies: size line 4096 bytes, 16 hex digits, extension bytes
  bounded by the data, trailer section 4096 bytes, 64 trailers kept.
- Request body: 64 MiB (XO_HTTP_MAX_BODY, with_max_body); over it the
  connection closes (the writer's rule since 1f077021f; eb10d0186 made
  its Connection header follow Go 1.26.11: written after the handler's
  fields, none on HTTP/1.0).
- HTTP/2: 250 concurrent streams, 250 running and 1000 queued handlers,
  header list 1 MiB + 32 per field, a CONTINUATION fragment at most twice
  what the list may still hold, empty CONTINUATION frames not decoded
  (f2c99600d), HPACK table 4096; unfinished buffered request bodies at
  most four times the body limit per connection, the stream past it
  refused with REFUSED_STREAM (d4cc39fe0; Go bounds this with
  MaxReceiveBufferPerConnection; a withheld WINDOW_UPDATE is the
  follow-up that would make it flow control instead).
- Query parameters, forms: 10000, counted before splitting.
- Cookies 3000; redirects 10; Range 200 ranges.
- Multipart: 1000 parts, 10000 headers, 10 MiB of header bytes checked
  while read, 32 MiB + 10 MiB of memory; a part's header read in linear
  time (4d5b1431a).
- textproto read_header: linear in the number of fields (390951579;
  40000 fields took 5.8 s, now 3 ms); std/mail and std/smtp use it on
  network input.

## 3. Constant time

std/http compares no secrets. Cookie values and Authorization are
passed through, not checked; authentication is the handler's.

## 4. Fault behaviour

The diff tests and fuzz harnesses above give Go's answer or Go's error
for every input, with no fault, on arm64, llvm and go. Follow-up runs,
recorded as their own evidence files: race.md, stress.md (the program
std/evidence/http/stress: 64 client tasks, slow readers, half sent
heads, resets, oversized heads, CL with TE, garbage), fuzz.md, perf.md.

## 5. Defaults and intended differences

- A request with both Transfer-Encoding: chunked and Content-Length is
  framed by chunked and the connection closes after the answer (RFC
  9112 6.3, a MUST); Go keeps the connection open (be245b2cf).
- A Host with CR or LF fails before connecting; Go's HTTP/1 sends an
  empty Host. Newlines in User-Agent become spaces, as in every other
  field; Go's Transport rejects the value.
- An HTTP/2 HEADERS or CONTINUATION fragment more than twice what the
  header list may still hold ends the connection with GOAWAY
  PROTOCOL_ERROR. Go up to 1.26.1 did the same; Go 1.26.11 keeps the
  connection open and reads on. Xo keeps the close: such a block can at
  best end in 431, and reading it only spends the server's CPU (the
  CVE-2024-27316 class). TestH2Diff records it (h2Intended).
- Request fields: 10000 at most, beside Go's byte limit.

## 6. Second reader's findings

Lang 2 read the stack before it landed; each finding was fixed in it.

1. An empty CONTINUATION that does not end the block was decoded as a
   fragment (an effect of the GO-2023-1571 fix). Lang 2 measured 20000
   of them after 800000 waiting bytes at 825 ms (about 41 us a frame);
   aws-43's golden, 50000 after 800 KB of a value, took 2.2 s before
   the fix and 3 ms after. Fixed in f2c99600d. The fix's first version
   also skipped an empty HEADERS, which must still open its block; Lang
   2's follow-up read found that regression, fixed in the same commit
   before it landed; golden http_h2_empty_continuation has both.
2. The HTTP/2 client kept a Content-Length past 2^63 - 1. Fixed in
   7005ae3d0, as Go's ParseUint.
3. The 100 Continue state (X_S100) was changed by the reader and the
   writer with no lock, flagged as unverified (a race by reading, not an
   interleaving seen on the wire). Fixed in
   968197a15 (compare and swap, a head waits out a 100 being sent; on
   wasm, where a server's tasks run one at a time, no wait).
4. multipart's part header was quadratic in its fields (1364 ms for the
   case, 13 ms after). Fixed in 4d5b1431a.
5. Request.query_all split before counting on every backend. Fixed in
   26bf45db0.
6. The Go backend's Fs on Windows reached device names (NUL, CON). Fixed
   in 1759348e9 (filepath.IsLocal), checked on the windows/arm64 VM.
7. A streamed CONNECT (open) was pooled. Fixed in 33b732005.
8. ReverseProxy forwarded a repeated key's values as sent
   (`a=1&a=evil` raw), not the clean query; fixed in 3c079900e, its doc
   and security entry in add000f9a.
9. std/changes/url.md's GO-2026-4341 entry said the Go backend used
   net/url's ParseQuery and was not affected; the Go backend runs the
   generated port of parse_query, so every backend was. Corrected with
   the query_all fix (26bf45db0).
10. The fixes had no CHANGELOG Security lines (which the 0.51 notes
    build from). Added in e5a057040.

The gate found two more defects in the fixes themselves: the 100
Continue wait did not build for wasm (fixed before landing), and the Go
backend's HTTP/2 field check took an HTTP/1 status line reading
HTTP/2.0 for HTTP/2 (it now asks the TLS handshake, b340e6ebd).

Also checked and not vulnerable: an HTTP/2 peer that never reads is
bounded by the write timeout (write_byte_timeout), whose default is
Go's.

## Sign-off

First reader: aws-43, 2026-11-13, every advisory read against the code
at f557f6e3d; the fixes as listed, all on main by 849937377 (this
file's commit and source, the 1M-case fuzz run's).

Second reader: Xo Lang 2, 2026-11-13, sections 1-6 read; findings fixed
as listed.
