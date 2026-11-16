# std/jwt against golang-jwt: where the fuzz accepts another answer

TestJwtFuzz (internal/stdtest/jwtfuzz_test.go, cases from
scripts/stdcorpus/jwtoracle's fuzz mode) checks that std/jwt gives
golang-jwt v5.3.1's error category for every token. In these cases
the oracle lists more than one acceptable category ("a|b" in the want
field), and the case's note says why ("also ..."). Found 2026-11-16 at
seeds 2 to 8. None was an Xo bug.

- **Joined errors.** golang-jwt checks every claim and joins the errors.
  The oracle reports them in a fixed priority, while std/jwt stops at the
  first failure, so any of the joined categories is accepted.
- **Missing iss, aud or sub** when one is required. golang-jwt reports a
  missing claim; std/jwt documents it as Issuer, Audience or Subject.
- **A signature that is not base64url.** golang-jwt says malformed;
  std/jwt documents Signature.
- **A key that does not fit the header's alg** (for example an HMAC
  secret for ES256, or the P-384 key for ES256). golang-jwt says invalid
  signature; std/jwt refuses the algorithm (alg confusion).
- **`crit` in the header.** golang-jwt ignores it; std/jwt refuses it as
  RFC 7515 4.1.11 requires.
- **A header or payload that is not UTF-8.** Go's JSON replaces the bad
  bytes; std/jwt's JSON refuses them as malformed.
- **An `iat` that is not a number.** golang-jwt reads iat only with
  WithIssuedAt; std/jwt documents a time claim of the wrong type as Claim.
- **A numeric date of 2^63 seconds or more.** It wraps Go's time.Time,
  making a token expired or valid at random; std/jwt says Claim.
- **`exp: 0`.** golang-jwt reads 0 as absent. RFC 7519 makes it a time,
  so std/jwt says expired, or ok while now is within the leeway of it.

The oracle also decides "alg" first from the header decoded as JSON (a
repeated key's last value, as golang-jwt reads it): an alg that is
missing, not a string or not allowed. std/jwt checks alg before the
claims; golang-jwt decodes the claims first.
