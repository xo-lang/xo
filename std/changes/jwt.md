# std/jwt changes

## 2026-11-15: test: go-jose decrypts the tokens std/jwt encrypts

- Kind: test
- Commit: this push
- Found by: decision 0660's acceptance (owner, 2026-11-15): a Go side test in the reverse direction before jwt features count as Proven (task #39)
- Go reference: github.com/go-jose/go-jose/v4 v4.1.9 ParseEncryptedCompact and Decrypt
- Test: internal/stdtest TestJweGoJoseDecrypts: jweoracle `reverse` prints an Xo program that encrypts under every key algorithm and content encryption (96 combinations; empty, short and long payloads, deflate, typ and kid), and jweoracle `decrypt` decrypts each token with go-jose and checks the payload and kid; 96 of 96 on arm64, llvm and go
- Shipped in: not yet released
- Timing: none

No behavior change: until now the corpus checked go-jose's tokens in
std/jwt and std/jwt's tokens in std/jwt; go-jose now reads std/jwt's.

## 2026-11-15: feature: JWE compact tokens

- Kind: feature
- Commit: this push
- Found by: the stdaudit features line (decision 0655): jwt.behavior.jwe; decision 0660
- Go reference: github.com/go-jose/go-jose/v4 v4.1.9 (jwe.go, crypter.go, symmetric.go, asymmetric.go, cipher/; Apache 2.0)
- Test: internal/stdtest/testdata/corpus/jwe (scripts/stdcorpus jwe, run by scripts/stdcorpus/jweoracle): 96 go-jose tokens over every key algorithm and content encryption, with wrong keys and a changed character in each part, 615 cases decrypting or failing as go-jose's Decrypt; every combination encrypted by std/jwt and decrypted back (go, arm64, llvm)
- Shipped in: not yet released
- Timing: none

`encrypt(r, alg, enc, key, payload, opts)`, `decrypt(token, key, algs,
encs)` and `peek_jwe_header(token)`; `JwtErr` gains `Decrypt` (an open
enum, so a match with a wildcard arm keeps working). AES key wrap
(RFC 3394) and the Concat KDF are written in std/jwt over std/crypto's
AES block, GCM, CBC, HMAC, PBKDF2, OAEP and ECDH.

## 2026-11-15: feature: Options.all_audiences and Options.require_nbf (golang-jwt's WithAllAudiences, WithNotBeforeRequired)

- Kind: feature
- Commit: this push
- Found by: scripts/stdaudit gaps jwt.WithAllAudiences and jwt.WithNotBeforeRequired (decision 0655)
- Go reference: github.com/golang-jwt/jwt/v5 v5.3.1 validator.go verifyAudience and verifyNotBefore, parser_option.go (MIT license)
- Test: std/jwt_test.xo, the cases of validator_test.go Test_Validator_verifyAudience and Test_Validator_verifyNotBefore and map_claims_test.go's all-audience row (arm64, llvm; corpus jwt on go)
- Shipped in: not yet released

Every name in `all_audiences` must be in `aud` (a string or a list of strings), else `Audience`; with `require_nbf` a token without `nbf` is `Claim("nbf is required")`. Both default off: tokens that verified before still do.

## 2026-11-15: feature: `JwtErr` is an open enum (decision 0649)

- Kind: feature
- Commit: this commit
- Found by: decision 0649 (a new variant of a std error enum broke two SDKs, 0644's `ClientErr.Tls`)
- Go reference: none (Go's errors are values compared with errors.Is; Rust's `#[non_exhaustive]`)
- Test: compiler/check's open enum test; the open_enum golden; xo check over the programs that use the module

`JwtErr` is declared `open`: a `match` on it outside std needs a `_` (or
binding) arm, warning XO0306 in 0.52 and an error after, so that std can
add a variant in a minor release without breaking a build. Matches that
already have `_` are unchanged.

## 2026-11-10: refactor: one file with its bodies (decision 0588 stage 2)

- Kind: refactor
- Commit: this commit
- Found by: decision 0588 stage 2 (pure modules merged with their implementation)
- Go reference: none (no behavior change)
- Test: std/jwt_test.xo (`xo test --std-module=jwt` on native and llvm); internal/stdtest corpus jwt on go, arm64 and llvm

std/jwt.xo now holds the bodies of std/native/jwt.xo (removed), and the Go
backend compiles them from it as its own package; std/rt/jwt.go (the Go backend's display methods) is removed, the
module's own `display` serves it. The API, texts and errors
are unchanged: every item the declaration file had is `pub`, with the same
parameter names.

## 2026-11-05: feature: extra protected header fields and x5c

- Kind: feature
- Commit: this commit
- Found by: the Azure SDK (azidentity's client assertion needs x5t#S256 and optionally x5c in the header)
- Go reference: RFC 7515 4.1.6 (x5c), golang-jwt Token.Header (any field)
- Test: jwt_test.xo "extra header fields and x5c follow alg, kid and typ in order of name"

`sign(..., header: Map[Str, Str] = {}, x5c: List[Str] = [])` writes the fields after `alg`, `kid`, `typ` in order of name; `Header` gains `extra` and `x5c`. Existing calls are unchanged and write the same header. Reserved names are refused.

## 2026-11-04: fix: an aud list holding a non string is refused; docs

- Kind: fix
- Commit: this push
- Found by: the owner's v0.38.1 evaluation (doc mismatches)
- Go reference: golang-jwt v5 ClaimStrings (a string or a list of strings; another element is ErrInvalidType)
- Test: jwt_test.xo "an aud list holding a non string is refused when the audience is checked"
- Shipped in: not yet released

With `Options.audience` set, an `aud` of `["x", 1]` was accepted for `"x"`; it is now `Claim("aud is not a string or a list of strings")`. The `JwtErr.Claim` description names what it covers, against Issuer, Audience and Subject. spec/std.md 62 no longer says every token is byte for byte Go's: a claim text with `<`, `>` or `&` is written as it is, where Go's encoding/json escapes it (both verify alike).

## 2026-11-04: fix: exp, nbf and iat past 2262 no longer fault

- Kind: fix
- Commit: this push
- Found by: the owner's v0.38.1 evaluation (b25): a correctly signed token with `exp` 32503680000 (year 3000) killed the process
- Go reference: golang-jwt v5 (time.Time holds such dates)
- Test: jwt_test.xo "dates past 2262 do not overflow (b25)"
- Shipped in: not yet released

The claims are kept as whole seconds and compared in seconds (leeway and `max_age` added without overflow), where they were multiplied into a nanosecond Duration. The `at` of `Expired` and `NotYetValid` for a date a Time cannot hold is clamped to about the year 2255.

## 2026-11-03: feature: std/jwt

- Kind: feature
- Commit: this push
- Found by: the std coverage page (JWT, JOSE: planned); decision 0486, Accepted by the owner 2026-11-03
- Go reference: github.com/golang-jwt/jwt/v5 v5.3.1
- Test: internal/stdtest corpus jwt on arm64, go and llvm (generated from golang-jwt by scripts/stdcorpus/jwtoracle); std/native/jwt_test.xo
- Timing: bench/results/std-twin-jwt-2026-11-03.md
- Shipped in: not yet released

New module: JWS compact tokens for HS256/384/512, RS256/384/512, PS256/384/512, ES256/384/512 and EdDSA.

- `sign(r, alg, key, claims, kid)` builds `{"alg","kid","typ"}.claims.signature`; the corpus has golang-jwt's exact token for every deterministic algorithm.
- `verify_with(token, key, opts)` and `verify(token, keys_fn, opts)` check size, structure, the allowed `alg`, the key's fit, the signature, then the claims, and return the claims decoded as the caller's type.
- No `none`, no JWE, `crit` refused, `alg` list required.
- `peek_header` and `jwks` for key selection by `kid`.
