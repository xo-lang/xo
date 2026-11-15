# 0660: JWE compact tokens in std/jwt

Status: Accepted (owner, 2026-11-15; drafted by Xo Lang 3). Before jwt's features count as Proven, a Go side test decrypts std/jwt's tokens with go-jose (the other direction of the corpus).

## Context

The stdaudit features line (decision 0655) lists `jwt.behavior.jwe`:
std/jwt had JWS only (decision 0486). Go programs read and write JWE with
github.com/go-jose/go-jose (v4), for encrypted ID tokens, nested JWTs and
encrypted cookies. Every primitive JWE needs is in std/crypto (AES block,
GCM, CBC, HMAC, PBKDF2, RSA-OAEP, ECDH) except AES key wrap and the
Concat KDF, which are small.

## Decision

std/jwt gains `encrypt`, `decrypt` and `peek_jwe_header` for the compact
serialization (spec/std.md 62.1), after go-jose v4.1.9:

- Key algorithms: `dir`, A128KW/A192KW/A256KW, A128GCMKW/A192GCMKW/
  A256GCMKW, RSA-OAEP and RSA-OAEP-256, PBES2-HS256+A128KW,
  PBES2-HS384+A192KW, PBES2-HS512+A256KW, ECDH-ES and ECDH-ES+A*KW on
  P-256, P-384 and P-521. Content: A128CBC-HS256, A192CBC-HS384,
  A256CBC-HS512, A128GCM, A192GCM, A256GCM. `zip: DEF`.
- Not included: RSA1_5 (RFC 8725 3.2: do not use; go-jose keeps it for old
  tokens), the JSON serialization and several recipients, X25519 ECDH
  (go-jose v4 has none either).
- `decrypt` takes the allowed key algorithms and content encryptions with
  no defaults, as `Options.algs` for JWS (decision 0486). Every failure
  after the header and the allowed lists is one error, `JwtErr.Decrypt`
  (go-jose's ErrCryptoFailure), so a caller cannot learn which check
  failed (padding oracle, RFC 7516 11.5). go-jose's limits are kept: a
  PBES2 count above 1,000,000 fails, a `zip` payload inflates to at most
  max(250000, 10 times its size) bytes.
- `JwtErr` (an open enum) gains `Decrypt`.

Evidence: the jwe corpus. go-jose, with a constant random stream so the
corpus regenerates byte for byte, encrypts every key algorithm with every
content encryption; std/jwt must decrypt each token and fail on each
changed copy and wrong key exactly where go-jose's Decrypt does (615
cases), and decrypt what it encrypts for every combination, on the Go,
arm64 and LLVM backends.

## Consequences

- An encrypted or nested JWT is two calls (`encrypt` of the claims with
  `typ`/`cty` "JWT"; `decrypt`, then `verify` for a nested one); no claims
  helper for JWE yet.
- The corpus checks Xo against go-jose in one direction (go-jose's tokens
  into std/jwt); std/jwt's own tokens are checked by its round trip. A Go
  side test that decrypts std/jwt's tokens with go-jose is the next
  evidence step.
- `stdaudit` maps `jwt.behavior.jwe` to `std/jwt.encrypt`.

## Alternatives

- A separate std/jose module: rejected; the header, base64url and key
  handling are std/jwt's, and users look for JWE next to JWT.
- Fine grained decrypt errors: rejected for the padding oracle reason
  above, and go-jose gives one error too.
- Including RSA1_5 for old tokens: rejected (RFC 8725); can be added
  behind an explicit opt-in if a user needs to read such tokens.
