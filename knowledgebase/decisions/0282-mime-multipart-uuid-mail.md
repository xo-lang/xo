# 0282: std/mime, std/multipart, std/uuid, std/mail

Status: Accepted (owner, 2026-10-20). Proposed by Xo Lang Lib 1; built
after the Postgres stages of decision 0278. Each module's spec section
lands with it.

## Context

The survey of decision 0276 (knowledgebase/infrastructure/std-coverage.md)
ranked multipart uploads, MIME types, and content sniffing (the file
upload service), UUIDs (upload and CRUD services), and mail addresses
(the worker and auth services, with SMTP) after the Postgres driver and
the encodings of decision 0279. All but UUID are Go std packages; UUID
is github.com/google/uuid (BSD-3) and RFC 9562.

## Decision

Four modules, ported from Go with Go's tests (google/uuid's for std/uuid),
each with a std/changes/MODULE.md log; Xo Lang 3 reviews the std/http
part (Request.multipart_form). The API:

### std/mime (Go's mime: mediatype.go, type.go; net/http sniff.go)

```
struct MediaType { kind: Str, params: Map[Str, Str] }   // kind lower case: "text/html"
struct MimeErr { msg: Str }                              // Go's texts: "mime: invalid media parameter"
pub fn parse_media_type(v: Str) -> MediaType ! MimeErr   // RFC 2045 and 2231 (continuations, charsets)
pub fn format_media_type(kind: Str, params: Map[Str, Str]) -> Str   // "" when kind is invalid; RFC 2231 for non ASCII
pub fn type_by_extension(ext: Str) -> Str?              // ".png" -> "image/png": Go's builtin table only
pub fn extensions_by_type(kind: Str) -> List[Str]
pub fn detect_content_type(data: Bytes) -> Str          // net/http DetectContentType: the first 512 bytes
```

- `type_by_extension` uses Go's builtin table and no system files
  (`/etc/mime.types` would be an unattenuated file read): the same answer
  on every OS.
- std/http's `form()` then rejects a malformed Content-Type as Go does
  (`mime: invalid media parameter`), closing the gap noted in 4.3.

### std/multipart (Go's mime/multipart: multipart.go, formdata.go, writer.go)

```
struct Part { name: Str, filename: Str?, headers: Map[Str, Str], data: Bytes }
struct Form { values: url.Values, files: Map[Str, List[Part]] }
struct MultipartErr { msg: Str }
pub fn parse(body: Bytes, boundary: Str) -> List[Part] ! MultipartErr
pub fn parse_form(body: Bytes, boundary: Str, max_parts: Int = 1000) -> Form ! MultipartErr
pub fn write(parts: List[Part], boundary: Str) -> Bytes   // for clients and tests
pub fn random_boundary(r: Rand) -> Str uses rand
fn (r: http.Request) multipart_form() -> multipart.Form ! multipart.MultipartErr   // in std/http
```

- In memory: the body is the request's, capped by the server
  (`XO_HTTP_MAX_BODY`); streaming parts waits for a streaming request
  body (survey gap 9's other half).
- Limits as Go's (`max_parts`, header size, `Content-Disposition`
  parsing through std/mime); file names are returned as sent (the caller
  joins them under an `Fs.sub`, so a path cannot escape).

### std/uuid (RFC 9562; google/uuid's tests and vectors)

```
type Uuid struct(Bytes)                           // a newtype over 16 bytes; Eq, Ord, Hash, Display, Json (text)
pub fn v4(r: Rand) -> Uuid uses rand
pub fn v7(r: Rand, c: Clock) -> Uuid uses (rand, clock)   // time ordered: database keys
pub fn parse(s: Str) -> Uuid ! UuidErr             // 8-4-4-4-12, urn:uuid:, braces, 32 hex
pub fn nil() -> Uuid
fn (u: Uuid) version() -> Int
fn (u: Uuid) display() -> Str                      // lower case 8-4-4-4-12
```

- Randomness from `Rand` and time from `Clock`, so tests are seeded and
  reproducible; std/sql reads a Postgres `uuid` column with `str` and
  `uuid.parse` (no new SqlValue).

### std/mail (Go's net/mail: addresses only)

```
struct Address { name: Str, address: Str }
struct MailErr { msg: Str }
pub fn parse_address(s: Str) -> Address ! MailErr        // "Ann Lee <ann@example.com>", RFC 5322
pub fn parse_address_list(s: Str) -> List[Address] ! MailErr
fn (a: Address) display() -> Str                          // quoted and RFC 2047 encoded as Go's String()
```

- Message parsing (`ReadMessage`) and SMTP come with a later SMTP
  decision, which needs TLS on a connection (decision 0281).

### Tests

Go's tests for each package ported (mediatype_test, type_test,
sniff_test, multipart_test, formdata_test, writer_test, message_test's
address tables), google/uuid's for std/uuid, and goldens on every
backend; a differential test against Go on random inputs for
parse_media_type and parse_address.

## Consequences

- The upload service of the survey is writable with std alone (the body
  stays in memory, capped by the server, until a streaming request body
  lands); std/http's form() rejects a malformed Content-Type as Go does.
- MIME types come from Go's builtin table on every OS (no /etc/mime.types).
- UUIDs come from Rand and Clock, so tests are reproducible.

## Alternatives

- One std/mime module holding multipart: Go keeps them apart, and a Go
  reader looks for mime/multipart.
- Reading the system's MIME type tables: an unattenuated file read and
  answers that differ by OS.
- UUID as a SqlValue: a uuid column reads as Text and parses with
  uuid.parse, which keeps std/sql's value set small.
