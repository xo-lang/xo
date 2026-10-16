# The native HTTP client returns a gzip body undecoded

Status: Observed (2026-10-16), not fixed.

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

Either decode a gzip body when the client added the header itself
(std/native/archive.xo already inflates gzip natively), dropping
Content-Encoding and Content-Length as Go does, or stop sending the
header. Add a case to the client differential tests with a server that
answers gzip only when asked.
