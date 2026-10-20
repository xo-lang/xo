# std/tls changes

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
