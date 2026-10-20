# 0279: URL encoding, binary encodings, public hashes

Status: Accepted (owner, 2026-10-20): one module per Go package
(std/url, std/base64, std/base32, std/binary), with the std/http
additions agreed with Xo Lang 3. Spec: spec/std.md 20 to 22 and 11's
additions, landing with each module. Proposed by Xo Lang Lib 1.

## Context

The survey of decision 0276 ranked, after the Postgres driver (0278):
URL query and form encoding (3 of 5 services: OAuth2 token requests,
Twilio and Slack form posts, repeated query keys), base64 URL and raw
encodings, base32, and byte order helpers (2 of 5: JWTs, cursors, TOTP,
wire protocols), and hashes other than SHA-256 (2 of 5: HMAC-SHA1
webhook signatures, TOTP, streaming a large upload's digest). Each is a
small Go package with stable behavior and good tests, and most of the
code already exists in std/native/llrt for TLS (SHA-1, SHA-224 to
SHA-512, SHA-3 with streaming hashers; base64 URL in digest.xo), without
a public API.

## Decision

1. **std/url**: `query_escape`, `query_unescape`, `path_escape`,
   `path_unescape`, and `Values` (an ordered multimap with `parse_query`
   and `encode`), ported from Go's net/url (url.go, go1.26), with Go's
   tests (url_test.go's escape and query tables). std/http gains
   `Request.form()` and `Request.query_all(name)` on top of it, agreed
   with the HTTP owner; `Url.query` keeps its type.
2. **std/base64 and std/base32**: `encode` and `decode` with the four
   RFC 4648 alphabets each (`Std`, `Url`, `RawStd`, `RawUrl`; `Std`,
   `Hex`, `RawStd`, `RawHex`), Go's decoding rules and error offsets,
   Go's test tables. The prelude's `to_base64` and `from_base64` stay.
3. **std/binary**: byte order writes (`u16_be` ... `u64_le`, returning
   Bytes) and reads (`get_u16_be` ... with signed forms), and varints,
   as pure functions; the native backends compile them to loads and
   stores.
4. **std/crypto**: `Hash` (MD5, SHA-1, SHA-224, SHA-256, SHA-384,
   SHA-512, SHA3-256, SHA3-512), `digest`, `hmac` over any of them, and
   a streaming `Hasher` value (`hasher`, `hmac_hasher`, `write`, `sum`).
   Native builds expose the TLS stack's implementations; MD5 is ported
   from Go's crypto/md5. The weak hashes are offered for the protocols
   that require them, with an `xo vet` note when used outside an HMAC.
5. **Names follow Go's** (module per Go package, `query_escape` for
   QueryEscape, `RawUrl` for RawURLEncoding), per decision 0276 item 4;
   errors are typed results with Go's texts.
6. **Tests and change logs.** Each module ports Go's tests for it and
   gets a differential test against Go on random inputs; each lands
   with its `std/changes/MODULE.md` entry (kind `port`, Go reference
   file and version).

## Consequences

- OAuth2, JWT (HS256), TOTP, form posts, and webhook signatures are
  writable without hand rolled escaping or encoders.
- The Postgres driver (0278) uses std/binary for its message framing
  and `crypto.Hash.Md5` for MD5 authentication.
- Four new small modules in std's surface; their APIs freeze with the
  first edition.

## Alternatives

- **One std/encoding module** holding base64, base32, binary, and hex.
  Fewer modules, but Go readers look for each package by its name, and
  `base64.encode` reads better than `encoding.base64_encode`.
- **Methods on Bytes in the prelude** (`b.to_base64_url()`). Grows the
  prelude for every variant; the prelude keeps the common Std form only.
- **A Hash interface that user types implement.** Not needed by any
  survey service; the closed enum keeps `digest` and `hmac` simple and
  lets the native backends pick the CPU's instructions.
