# 0486: std/jwt: JSON Web Tokens on our crypto, with golang-jwt's protections

Status: Proposed (Xo Lang 3, 2026-11-03), at the Lead's request after decision 0479. Redis is a separate module and
its own decision.

## Context

Backends that issue or accept bearer tokens write JWT handling by hand today: split on `.`, `base64.decode_url`,
`json.parse`, `crypto.hmac` or `crypto.verify_raw_ecdsa`, and check `exp` themselves. The failures of such code are
well known (accepting `alg: none`, verifying an RS256 token with the public key as an HMAC secret, comparing
signatures with `==`, ignoring `exp`, trusting `kid` or `jku`). Go's ecosystem answer is golang-jwt/jwt v5. std has
every primitive: `crypto.hmac`, `crypto.equal`, `crypto.verify` and `sign` (RSA PKCS #1, ECDSA, Ed25519),
`verify_pss`/`sign_pss`, `verify_raw_ecdsa`/`sign_raw_ecdsa` (JWS's fixed size r || s), `jwk_public_key`,
`base64`'s URL alphabet, and `json`.

## Decision (proposed)

A module `std/jwt` (JWS compact serialization, RFC 7515 and RFC 7519; no JWE, no JSON serialization).

```
enum Alg { HS256 HS384 HS512 RS256 RS384 RS512 PS256 PS384 PS512 ES256 ES384 ES512 EdDSA }   // no "none"

enum Key {
  Hmac(secret: Secret[Bytes])
  Public(key: crypto.PublicKey)
  Private(key: crypto.PrivateKey)
}

struct Header { alg: Alg, kid: Str?, typ: Str? }

struct Options {
  algs: List[Alg]                    // required: the algorithms this call accepts
  now: Time                          // the clock is passed in (tests, replay), never read inside
  leeway: Duration = 0s              // exp, nbf and iat
  issuer: Str? = None                // required `iss`
  audience: Str? = None              // `aud` (a string or a list) must contain it
  subject: Str? = None
  require_exp: Bool = true
  max_age: Duration? = None          // against iat
  max_size: Int = 16384              // bytes of the token
}

enum JwtErr {
  Malformed(msg: Str)                // not three segments, bad base64url, bad JSON, duplicate member
  Alg(msg: Str)                      // none, not in algs, or a key that does not fit the alg
  Signature
  Expired(at: Time)  NotYetValid(at: Time)  Issuer  Audience  Subject
  Claim(msg: Str)                    // a registered claim of the wrong type
  Key(msg: Str)                      // no key for kid
}

fn sign[T: Json](alg: Alg, key: Key, claims: T, kid: Str? = None) -> Str ! JwtErr
fn verify[T: Json](token: Str, keys: fn(Header) -> Key ! JwtErr, opts: Options) -> T ! JwtErr
fn verify_with[T: Json](token: Str, key: Key, opts: Options) -> T ! JwtErr    // one key, no kid lookup
fn peek_header(token: Str) -> Header ! JwtErr      // for kid routing; unverified, documented as such
fn jwks(set: Str) -> Map[Str, Key] ! JwtErr        // RFC 7517 key set by kid, public keys only
```

The claims type is the caller's `derive Json` struct (so extra claims are typed); the registered claims `exp`,
`nbf`, `iat`, `iss`, `aud`, `sub` are checked by the module from the payload object before it decodes `T`, so a
`T` without them still gets them validated.

Protections (each with a negative corpus row):

1. **No `none`.** `Alg` has no such variant; a header `alg` outside it is `Alg`.
2. **Caller fixes the algorithms** (`Options.algs`, no default). The token's `alg` is only matched against it.
3. **Key must fit the algorithm** (golang-jwt's `ErrInvalidKeyType`): HS* only with `Key.Hmac`, RS*/PS* with an RSA
   public key, ES256/384/512 with P-256/384/521 respectively, EdDSA with Ed25519. An RSA public key presented as an
   HMAC secret is impossible by construction (`Hmac` takes a `Secret[Bytes]`, never a `PublicKey`'s bytes), which
   is the classic RS256 to HS256 confusion.
4. **Constant time** HMAC comparison (`crypto.equal`).
5. **Strict encoding**: base64url without padding, no whitespace or extra segments, a header and payload that are
   JSON objects with no duplicate members, `crit` present is `Malformed` (no extension is understood),
   `max_size` before any decoding. `jku`, `jwk`, `x5u` and `x5c` headers are ignored, never fetched.
6. **Claims**: `exp` required by default, `nbf`, `iat` and `aud` (string or array of strings) as RFC 7519 4.1, with
   `leeway`; a non-numeric date or non-string `iss` is `Claim`. NumericDate fractions are kept.
7. **Time is an input** (`Options.now`), so the module is pure and tests replay a clock.
8. **Signing**: ECDSA and RSA-PSS take their randomness from `Rand`: `sign` takes `r: Rand` as its first argument
   for those (HMAC and Ed25519 are deterministic); written `sign(alg, key, claims, r, kid)` with `r` required only
   by the effect `uses rand`.

## Consequences

- A backend verifies a token in one call with the protections on by default; hand-rolled code is no shorter.
- New public API in the std index, spec/std.md, the coverage page, `std/changes/jwt.md`; a corpus against
  golang-jwt/jwt v5 (Go oracle, `scripts/stdcorpus/jwt.go` with a vendored module for the oracle only).
- `Options.now` is explicit: callers pass `os.clock.now()`. This is the cost of purity; a `verify_now` convenience
  could be added later.

## Measurement plan

Twin `bench/perf/jwt` against golang-jwt/jwt v5 (Go module in the twin only): HS256 sign and verify, RS256 verify
(2048-bit), ES256 sign and verify, EdDSA sign and verify, a 5-claim payload each; host 1, `xo bench --ab`; target
at most about 1.05 of Go's wall (decision 0461), most of the time being the crypto already measured (x509 rows
0.98). Memory flat per call.

## Test plan

1. Round trip every `Alg` with Go: tokens signed by golang-jwt verify here and the reverse.
2. A negative corpus from golang-jwt's tests and the known attacks: `alg: none` (any case), HS256 with the RSA public
   key PEM as secret, a P-384 key for ES256, `crit`, padded base64, a fourth segment, duplicate `alg`, oversized
   token, `exp` just past and just before with leeway, `aud` as array, `nbf` in the future, wrong `iss`.
3. `kid` routing through `verify` with a `jwks` set, including an unknown `kid`.

## Alternatives

- **Port golang-jwt wholesale** (`Token`, `Keyfunc`, `Claims` interface, `SigningMethod` registry): its registry
  and `Keyfunc` returning `any` are the source of the key-type bugs; typed `Key` and a closed `Alg` remove them.
- **Only helpers on std/crypto** (`crypto.jws_sign`): no claims validation, which is where the mistakes are.
- **Do nothing**: every service re-implements the checks above.

## Open questions for the owner

- Include JWE (`dir`, `A256GCM`)? Proposed: no, later and separate.
- `verify_with` plus `verify` with a key function, or only the latter?
