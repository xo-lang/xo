# 0473: std/otp, HOTP and TOTP one time passwords (proposal)

Status: Accepted (owner, 2026-11-03), with a change: `verify` returns the matched time step

## Context

The 2026-10-30 survey (std-coverage.md, third party table: TOTP, pquerna/otp; the auth service of the
backend corpus) found that a service adding a second factor has to write RFC 4226's dynamic truncation and
RFC 6238's time step itself, plus the constant time comparison and the `otpauth://` URI an authenticator
app scans. The parts exist (std/crypto `hmac` with Sha1, Sha256 and Sha512, `equal` constant time,
std/base32, std/rand), but the 15 lines that join them are easy to get wrong (a timing leaking compare,
a counter encoded little endian, the code not zero padded). pquerna/otp is the Go package the corpus used.

## Options

1. `std/otp`, pure functions over Bytes secrets (the raw key, not its base32 text):

   ```xo
   pub fn hotp(secret: Bytes, counter: Int, digits: Int = 6, algo: Hash = Hash.Sha1) -> Str     // RFC 4226
   pub fn totp(secret: Bytes, unix: Int, period: Int = 30, digits: Int = 6, algo: Hash = Hash.Sha1) -> Str   // RFC 6238
   pub fn verify(secret: Bytes, code: Str, unix: Int, period: Int = 30, digits: Int = 6, algo: Hash = Hash.Sha1, skew: Int = 1) -> Int?
   pub fn generate_secret(r: Rand, len: Int = 20) -> Bytes uses rand                                     // crypto random
   pub fn uri(issuer: Str, account: Str, secret: Bytes, period: Int = 30, digits: Int = 6, algo: Hash = Hash.Sha1) -> Str   // otpauth://totp/...
   ```

   `digits` is 6 to 8 (a fault otherwise), `algo` Sha1, Sha256 or Sha512 (a fault otherwise). `totp` is
   `hotp` of `unix / period`. `verify` accepts the step of `unix` and `skew` steps either side, compares each
   candidate in constant time, and returns false for a code of the wrong length or with a non digit (never a
   fault: it takes untrusted input). `uri` percent encodes the issuer and account and writes the secret as
   unpadded base32, as the Key URI Format of the Google Authenticator wiki and pquerna's `Key.URL`.
   Cost: std/otp.xo declarations, one pure native source (about 60 lines, std/go_compiled.txt), spec/std.md
   section, std/changes, corpus (RFC 4226 appendix D and RFC 6238 appendix B vectors for all three
   algorithms, plus seeded random secrets, counters and times against an independent Go implementation on
   crypto/hmac, and `verify` windows), a timing row against that Go code.
2. Only `hotp` and `totp`: smaller, but leaves the timing safe compare and the skew window to every caller,
   the mistakes this module exists to prevent.
3. A Key type holding issuer, account, secret and parameters (pquerna's `Key`): a second shape for one URI;
   rejected, the five functions cover its uses.

## Recommendation

Option 1. Names follow RFC 4226 and 6238 (hotp, totp), `verify` is the window check pquerna's `ValidateCustom`
does with one default (skew 1), and the secret is Bytes so a caller decides how it is stored.

## Consequences

A new public module (hence this ADR), pure but for `generate_secret`'s `uses rand`. Time is an Int of Unix
seconds so a test passes a fixed instant and the module needs no clock capability. A used code is not
remembered: replay protection (reject a code already accepted for the step) belongs to the service that
stores state.

## Alternatives

Listed above (2, 3). Also rejected: a base32 text secret as the parameter (the module would decode on every
call and callers keep raw keys anyway); a clock inside `verify` (untestable without a fake).

## Owner decision (2026-11-03)

Accepted with one change (Xo Lead's suggestion): `verify` returns the matched time step (`Int?`: the counter
`unix / period + offset` that matched, `None` when no step in the window matches) instead of `Bool`, so a caller
can store the last step used and reject a replay of the same code (RFC 6238 section 5.2). Every candidate step is
still compared, in constant time, before the result is chosen. A test checks that a second `verify` of the same
code returns the same step, which is how a caller detects the replay.
