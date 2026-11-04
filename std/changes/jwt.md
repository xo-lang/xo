# std/jwt changes

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
