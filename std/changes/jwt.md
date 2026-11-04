# std/jwt changes

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
