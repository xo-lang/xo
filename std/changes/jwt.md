# std/jwt changes

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
