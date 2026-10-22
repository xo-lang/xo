# std/tls changes

## 2026-10-21: speedup: a TLS client handshake on macOS without a zone lookup or slow symbol lookups

- Kind: speedup
- Commit: 7b590102
- Found by: Xo lang 4's profile of the arm64 TLS client (open and readlink in every handshake)
- Go reference: none
- Test: internal/llvm/testdata/golden/tls_client, tls13_go, tls12_client, http_tls (the expired certificate texts in time.Local); a local Go server with a self-signed certificate, 500 dials verified by the system (Mac)

Every client handshake (std/tls and std/http's https) read the
/etc/localtime link and looked up the local zone, which only a failed
verification's message uses; it is found now when that message is made.
On macOS the Security framework's functions are looked up through its
handle, opened once, where each lookup searched every loaded image (40
to 51 us a symbol, a dozen a handshake). 500 dials whose verification
fails on the Mac: 506 to 154 ms of CPU, 1,570 to 394 ms of wall time.

## 2026-10-20: feature: a TLS client on a dialed connection

- Kind: feature
- Commit: dfc2ef4e
- Found by: survey
- Go reference: crypto/tls Client, Config.InsecureSkipVerify and VerifyPeerCertificate; crypto/x509 CertPool.AppendCertsFromPEM (go1.26.1)
- Test: internal/llvm/testdata/golden/tls_client (arm64, LLVM, and Go agree); std/native/tls_test.xo; cmd/xo TestVetVerifyNone; scripts/pgtls/run.sh (postgres:16 in Docker: verify-full, verify-ca, require; arm64 and Go agree)

`tls.client(conn, host)` upgrades a connection the program dialed to TLS
(Postgres's SSLRequest, SMTP STARTTLS, Redis), with `Verify.Full` (chain
and host name), `ChainOnly` (libpq's verify-ca), or `None` (encryption
only, reported by `xo vet` as XO0902), and private roots with
`tls.roots(pem)`.
