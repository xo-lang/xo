# Go verifies system roots through macOS's Security framework

Status: Observed (2026-10-09, decisions 0105 and 0106)

Symptom: an https request with the system's roots to a server whose test
certificate is valid for 100 years failed on the Go backend with
`x509: “good” certificate is not standards compliant`, and a fresh day
long certificate of an unknown CA with `x509: “stray” certificate is not
trusted`, where Linux (and the native client before the fix) says
`x509: certificate signed by unknown authority`.

Cause: with `RootCAs` nil on macOS, crypto/x509's `Verify` hands the
chain to `SecTrustEvaluateWithError` (root_darwin.go `systemVerify`) and
only maps three error codes (expired, host name mismatch, not trusted) to
Go's own errors; any other code becomes `x509: <Apple's description>`.
Apple rejects TLS server certificates valid for more than 825 days before
anything else, and reports some untrusted chains with other codes.

Fix: the native client does the same on macOS: with the system's roots
`std/llrt/xo_tls.c` (`apple_verify`) calls the same Security framework
functions and maps the codes as Go does, so the texts match. Tests: the
`http_tls` golden prints only the variant of its system roots call
(its certificates are long lived, the text depends on the OS);
TestTLSDiff compares the full text with a certificate valid for a day
made at test time. With custom roots (`with_roots`) Go's own verifier
runs on every OS and the texts are the same everywhere.
