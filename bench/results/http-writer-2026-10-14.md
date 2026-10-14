# The HTTP/1.1 response writer in Xo (zero-C audit B12), 2026-10-14

Work item B12 of `zero-c-audit-refresh-2026-10-13.md`: xo_http.c's
response writer (lines 274-341 and 1398-1866 at 91aeb87d, 537 lines)
moves to Xo.

## What moved

- **std/native/llrt/http1w.xo** (new, beside http1.xo): net/http's
  StatusText, DetectContentType (`sniff`), the Date header
  (http.TimeFormat from `sysio.real_ns`, computed per response; the C
  cached it per second in a thread local), Header.writeSubset
  (`fields`: sorted by name, invalid names left out, values trimmed,
  newlines to spaces), the status line, chunkWriter.writeHeader's rules
  with std/rt's serve (`response`: Content-Length, Content-Type,
  Connection, Transfer-Encoding, the chunked framing, keep-alive), and
  conn.serve's error answers (`error_answer`).
- **std/native/llrt/cabi_http.xo** (new): the exports `xo_x_hw_response`,
  `hw_fields`, `hw_error`, `hw_sniff`, `hw_date` (decision 0204).
  std/native/http.xo imports it, so only programs with std/http carry
  them; xo_http.c has weak definitions that fault for the others (B08's
  pattern).
- **xo_http.c** keeps thin wrappers that reserve the room in the
  connection's buffer and pack the request's facts into flags
  (`hw_response_x`, `hw_response`, `hw_error`, `hw_fields` for the
  client's request header, `h_sniff`, `h_date`) and `h_body_allowed`
  (one line, called by the C connection and HTTP/2 code).
- **HTTP/2**: xo_h2.c's `h2_head` takes the sniffed type and the Date
  from http1w.xo; `h_itoa` callers use `fmt_int` (cabi.xo).
- The C test programs http_diff.c and http_fuzz.c link the writer from
  the LLVM IR of a std/http program (internal/llvm `cOnlyHTTP`).

Lines: xo_http.c 3,925 to 3,523 (514 deleted, 112 added), xo_h2.c +2;
http1w.xo 951 lines, cabi_http.xo 37.

## Performance (decision 0180's bar)

bench/perf/httpparse/measure.sh (20,000 requests a workload from load/,
the minimum of 5 runs of instructions retired from `/usr/bin/time -l`)
on the shared Mac (darwin arm64, other agents running). Base: a copy of
91aeb87d (`git archive`, its own xo); after: this branch at 1f077021.

| Workload | arm64 base | arm64 after | ratio | LLVM base | LLVM after | ratio |
|---|---|---|---|---|---|---|
| `none` | 29,634,665 | 29,614,151 | 0.9993 | 27,535,447 | 27,565,834 | 1.0011 |
| `h1_get` | 2,284,982,365 | 2,286,172,889 | 1.0005 | 2,099,798,536 | 2,091,808,471 | 0.9962 |
| `h1_chunked` | 1,327,152,758 | 1,321,243,548 | 0.9955 | 1,190,907,814 | 1,162,414,144 | 0.9761 |
| `h1_url` | 3,048,122,170 | 3,018,974,821 | 0.9904 | 2,908,754,533 | 2,876,647,850 | 0.9890 |
| `h2_get` | 5,113,475,863 | 5,141,778,289 | 1.0055 | 4,881,147,171 | 4,891,759,620 | 1.0022 |
| `h2_varied` | 5,284,456,340 | 5,318,550,117 | 1.0065 | 5,021,414,762 | 5,035,015,671 | 1.0027 |

HTTP/2 was measured twice more, base and after interleaved
(`WORKLOADS="h2_get h2_varied"`): the base's own runs spread 1.2 percent
(h2_get arm64 5,113,475,863 to 5,173,834,436), and the minimum over the
three measurements of each side is h2_get 1.0009 arm64 (5,113,475,863 to
5,118,034,412) and 1.0000 LLVM (4,878,730,934 to 4,878,668,844),
h2_varied 1.0016 arm64 (5,284,456,340 to 5,293,065,932) and 1.0011 LLVM
(5,021,414,762 to 5,027,098,200): within the noise of TLS workloads.
HTTP/1.1 is 0.976 to 1.001 of the C. No regression past the noise.

## Tests

- `go test -short` over the repo; TestHTTPParseDiff (-short, 300 random
  inputs against net/http byte for byte, ASan and UBSan) and
  TestHTTPFuzz pass with the Xo writer linked into the C harness.
- `xo dev goldens --only=http_client,http_fault,http_server,http_stream,http_tls,http_wire,net_only,request_eq,std_error_display`:
  9 of 9 on arm64 and on LLVM (the Mac).
- linux/arm64 (EC2 queue request b12-tests, warm r8g instance
  i-EXAMPLE06, 1f077021): the same goldens 9 of 9 on arm64 and
  on LLVM; `go test -run 'TestHTTPParseDiff|TestHTTPFuzz|TestHTTP1ParseDiff|TestH2Diff|TestCryptoDiff' ./internal/llvm`
  ok (49.7 s).
