# Go 1.26's crypto/tls ignores Config.Rand; a constant byte reader makes it repeatable

Observed 2026-10-15 (TLS 1.2 in Xo, std/native/llrt/tls12.xo and the
tls12_client golden).

Symptom: a crypto/tls handshake run twice with the same `Config.Rand`
produces different bytes (randoms, ECDHE keys, signatures), so a trace
recorded from Go cannot be replayed against it, and the Go reference of
a golden cannot reproduce a recorded client or server.

Cause: since Go 1.26 the crypto packages ignore a caller's random
source (crypto/internal/rand CustomReader) unless
`GODEBUG=cryptocustomrand=1`. Even then, `randutil.MaybeReadByte`
consumes one byte at random before some reads, so a counter or seeded
stream shifts by a byte from run to run.

Fix: set `cryptocustomrand=1` (`os.Setenv("GODEBUG", ...)` at run time
works: the runtime watches GODEBUG) and use a reader that returns the
same byte everywhere (`p[i] = 0x11`), so what is read does not depend on
how much was skipped. The client's random and session id are then 32
bytes of that byte, its X25519 key the byte repeated, and its P-256 and
P-384 keys what `ecdh.Curve.GenerateKey` derives from it (the golden
carries them); ECDSA and RSA-PSS signatures repeat too. std/rt
internal_tls12.go restores GODEBUG after the op. Recipe for new traces:
crypto/tls's client and server over net.Pipe, TLS 1.2 only, session
tickets off, readers of 0x11 (client) and 0x22 (server), each side's
writes recorded and split where its Handshake returned.
