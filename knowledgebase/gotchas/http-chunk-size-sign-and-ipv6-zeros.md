# The C request parser: a chunk size of 2^63 or more, and IPv4 parts with leading zeros

Status: Observed (2026-10-13, TestHTTP1ParseDiff on the C path,
XO_LLRT_C=http1)

Symptom: two inputs answered differently from the Go backend by the C
parser (xo_http.c) and the same by the Xo one (http1.xo):

- `Transfer-Encoding: chunked` with a chunk size line `ffffffffffffffff`
  and less data: hb_chunked cast the size to int64 (negative) and copied
  a negative length (`nbuf_put`), an out of bounds read AddressSanitizer
  would report.
- `GET http://[::1.2.3.04]/`: the C called the platform's inet_pton,
  which takes the IPv4 part's leading zero on macOS; net/url (the Go
  backend) rejects it (400).

Cause: a signed comparison of an unsigned size; a platform function
standing in for Go's address parser.

Fix: the chunk size compare is unsigned now (xo_http.c). http1.xo checks
IPv6 with BIND's inet_pton6/inet_pton4 rules (no leading zeros), as Go
does; the C path keeps the platform's inet_pton, so TestHTTP1ParseDiff
leaves that input out on the C path. A test against the Go backend over
real connections finds what a comparison with plain net/http (and the C
that was written to match it) does not.
