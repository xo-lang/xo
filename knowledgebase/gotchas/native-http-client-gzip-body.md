# The native HTTP client returns a gzip body undecoded

Status: Observed (2026-10-16), fixed 2026-10-16: http1.xo's client loop
decodes the body (std.md 4.5); goldens http_gzip (HTTP/1.1) and http_tls's
"gzip" case (HTTP/2).

## Symptom

`http.client(os.net).get("https://example.com/")` built with
`--backend=arm64` or `--backend=llvm` returns status 200 and a 400 byte
body that starts `1f 8b 08 00` (gzip) with `Content-Encoding: gzip`. The
same program on the Go backend (plain `xo build`) returns the 577 byte
HTML, as do Go's `http.Get` and curl. Found by the binary size comparison
(bench/results/binary-size-native-2026-10-16.md), whose smoke test
printed the body length.

## Cause

std/native/llrt/hclient.xo `request` copies Go's Transport and adds
`Accept-Encoding: gzip` when the caller set no encoding (nor a Range, nor
HEAD), but nothing on the response side decodes it. Go's Transport
decodes the body it asked to be compressed and drops Content-Encoding and
Content-Length (net/http `DisableCompression` is false by default); the
Go backend gets that from net/http, the native client does not.

## Fix

The client loop in std/native/llrt/http1.xo decodes a non-empty gzip
body with std/archive's gunzip when it added the header itself (no
Accept-Encoding or Range of the caller's, not HEAD), on HTTP/1.1 and
HTTP/2 alike, and drops Content-Encoding and Content-Length. One
difference from Transport is left: an empty body keeps its headers (Go
drops them for an empty chunked or close delimited gzip body, and keeps
them for Content-Length 0 and HTTP/2's END_STREAM on the headers). The
decoder adds about 33 KB to a program that uses the client (https_get
2,447,144 to 2,480,160 bytes stripped); a server only program is
unchanged.
