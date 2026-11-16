# 0655: a module's features are Proven only with no open function level gap, and the outside Go std rows get an audit

Status: Accepted (owner, 2026-11-16; proposed by Xo Lang 3, 2026-11-15; amends 0461; owner approved the direction through the lead, 2026-11-15)

## Context

Decision 0461 gives each std module a features mark: Proven is "near full parity, or beyond Go". scripts/stdcov
read it from the Status column of `knowledgebase/infrastructure/std-coverage.md` alone: every row naming the
module done meant Proven. That Status is set by hand, and the bar was each row's hand written Missing list.

Since 2026-11-10 scripts/stdaudit compares every exported symbol of the Go packages the table maps with Xo's
public surface, and scripts/stdaudit/map.txt records each as mapped, not planned, or `gap:`. stdcov did not read
it, so a row could say done while the audit listed open gaps in its package: unicode/utf8 (DecodeLastRune,
FullRune) and path/filepath (Rel, EvalSymlinks) are the cases a side session found (2026-11-15).

The 27 rows for modules outside Go's standard library (grpc, protobuf, websocket, amqp, otlp, jwt, cbor, sql and
others) have no audit at all: map.txt has no entry for them, so their marks rest on the Missing list alone. The
grpc row was stale: it listed compression and `-bin` metadata as missing although both landed on 2026-11-06,
and it never listed the health service or `unix:` targets.

## Decision

1. **Features Proven needs a clean audit.** stdcov marks a module's features Proven only when every coverage
   row naming it is done and the Go packages of those rows have no open `gap:` line in
   scripts/stdaudit/map.txt (the key's package is the last element of its import path, as map.txt keys it). A
   module with all rows done and open gaps is Solid, and its note names the gaps (the first four, then a count)
   and links map.txt. stdcov's TestFeatAuditGaps holds it.
2. **map.txt stays current.** A change that closes a gap maps the symbol in the same commit (four had landed and
   were still listed on 2026-11-15: tls.Server, tls.Listen, x509.CreateRevocationList, url.JoinPath). A
   wildcard gap key (`pkg.*`) is for a whole package that is missing; a gap in part of a package names its
   symbols (cookiejar.* became cookiejar.Options and cookiejar.PublicSuffixList).
3. **The outside rows get the same audit** (the next step of this decision): stdaudit pins each reference
   module (grpc-go with grpc/health, protobuf-go, gorilla/websocket, go-amqp, golang-jwt, the OpenTelemetry
   exporters, and the reference of each other outside row), lists its exported API as it does for Go's
   packages, and map.txt maps those symbols. Until a row has its audit, its module's features mark cannot be
   Proven.
4. **A behavior checklist per outside row.** A symbol diff misses protocol behavior, so each outside row gets a
   short checklist of what its reference implementation does that an API list does not show, each item done or
   missing. For grpc: health service, reflection, keepalive, interceptors, `unix:` targets, mTLS, name
   resolution. An open item counts as a gap.
5. **A module with no Go reference** (sse, go, amqptest, deque) gets its features bar from spec/std.md's declared
   surface plus a behavior checklist: its outside.txt line names `spec/std.md -` and its map.txt checklist key (lead,
   2026-11-15). Every std module has a row: a Go std package where one exists, else the Go library its port
   followed (yaml.v3, go-toml v2, x/time/rate, cenkalti/backoff v4, x/term, x/net httpproxy and publicsuffix, the
   MCP go-sdk's jsonrpc, protowire).
6. **Performance of an internal helper module** (htmlent, uniclass, protowire) is the lowest perf mark among the std
   modules that use it (html and htmltok; regexp; protodesc, protojson and protowkt); no twin of its own (lead,
   2026-11-15; scripts/stdcov perfHelpers).
7. **Performance of a test-only or bridge module** (addendum, Accepted by the lead for the owner, 2026-11-16 overnight; proposed by
   Xo Lang 2): twincheck runs a program, and these modules are used from tests or across a language boundary, so each
   has its own twin shape, held to the same 1.05 bar on host 1:
   - std/testing: a suite of 1,000 tests run by `xo test` against the same tests run by `go test`; the row is the
     harness overhead (the whole run less the tests' own work, which is the same trivial body on both sides).
   - std/amqptest: the fake broker's throughput (publish and consume of a fixed message set through it) against a
     Go twin of the same broker.
   - std/go (the `use go` bridge): the bridge call overhead (a call of a Go function with an Int, a Str and a
     Bytes argument and a result, in a loop) against a cgo call of the same shape.
8. **Not planned by category** (Accepted by the owner, 2026-11-16; proposed by Xo Lang 3 with task #110's triage):
   a map.txt row may close a gap as `not planned: CATEGORY: reason`, CATEGORY one of: `deprecated in Go`,
   `unsafe/reflection`, `Go runtime machinery`, `plug-in extension points`, `platform-specific`,
   `insecure legacy`, `design choice` (which must name its decision: `design choice: decision NNNN: ...`).
   stdcov counts such a row as resolved and names the count in the module's features text; a design choice
   without a decision number fails stdcov (TestNotPlannedCategory, TestFeatAuditGaps). The lead reviews each
   new one. The first use (2026-11-16): 206 rows of std/sql (pgx's geometric, tsvector, bit, xml, ltree and
   jsonpath types read as text, its codec interface, binary results, GSSAPI) and std/grpc (ClientConn's
   connectivity state); composite and domain types and LoadType are ported instead (task #113).

## Consequences

- The counts drop, and that is the point. At origin/main 7645bf7aa (`go run ./scripts/stdcov -today
  2026-11-15`): features 40 Proven and 50 Solid before, 22 and 68 after; overall 13 Proven before, 6 after
  (fnv, html, jpeg, json, norm and url keep it).
  - Open audit gaps (rule 1), eleven modules: archive (22 gaps), base32 (1), base64 (2), charset (6),
    compress (6), cookiejar (2), crc32 (1), csv (2), path (3), regexp (9) and time (8).
  - A done row outside Go's standard library with no audit yet (rule 3, stdcov counts it as `unaudited`),
    seven modules: bcrypt, cbor, cron, jwt, metrics, otp and uuid.
  - Overall Proven lost: base32, base64, compress, crc32, csv, time and uuid.
- Closing a listed gap (or recording it as not planned, with the reason) is now how a module earns the features
  mark back, and the gap lists in map.txt become a std work queue.
- The stdaudit rerun of 2026-11-15 (go1.26.11): 206 doc-named, 218 matched by snake_case, 2095 mapped, 8352
  skipped, 219 gaps, 0 unmapped (image/color/palette's Plan9 and WebSafe had been unmapped; they are gaps).

## Alternatives

- Keep the hand set Status and ask reviewers to read map.txt: the two files already disagreed, so a rule
  nobody runs is the current state.
- Count gaps by use (stdaudit -rank) and allow Proven below a threshold: "near full parity" has no agreed
  threshold, and a rarely used gap is cheap to close or to mark not planned with its reason.
- Leave outside rows without an audit: they are a third of the table and include the protocols most exposed to
  interop bugs.

## Implementation

- Rules 1 and 2 (2026-11-15): scripts/stdcov reads map.txt's gaps; map.txt corrected (four landed gaps mapped,
  palette and cookiejar keys).
- Rules 3 and 4, gRPC first (2026-11-15): scripts/stdaudit/outside.txt pins google.golang.org/grpc v1.79.2
  (the version TestGrpcInterop uses) with codes, status, metadata, health, keepalive and credentials; `-go`
  reads pinned packages from the module cache; map.txt maps all 307 symbols (113 gaps) and seven behavior
  checklist items (all open). stdcov reads outside.txt, so a pinned row's gaps apply and an unpinned done row
  stays `unaudited`; an outside row's "in progress" counts as partial.
- protobuf-go v1.36.12 (2026-11-15): proto, protowire, protojson and the seven well known type packages, 289
  symbols (protojson and the well known types whole-package gaps).
- gorilla/websocket v1.5.3 and golang-jwt v5.3.1 (2026-11-15): 71 and 133 symbols, with checklists (Autobahn,
  permessage-deflate, fragments; JWE).
- go-amqp v1.7.0 (2026-11-15): 134 symbols, keyed `go-amqp.` (the import path's last element); gaps SASL EXTERNAL
  and XOAUTH2. The amqp row was stale (connections, links and CBS had landed).
- otlptracehttp v1.47.0 (2026-11-15): 26 symbols (otlpmetrichttp has the same options; outside.txt takes one
  module a row) and a checklist (gRPC transport, partial success, Retry-After dates, logs). - bcrypt, uuid, cron, otp, cbor and client_golang (2026-11-15): golang.org/x/crypto v0.58.0 bcrypt (15
  symbols, no gap), google/uuid v1.6.0 (80), robfig/cron v3.0.1 (57; the scheduler not planned, decision 0474),
  pquerna/otp v1.5.0 with hotp and totp (47), fxamacker/cbor v2.9.0 (246) and client_golang v1.23.2 prometheus
  and promhttp (207). No done outside row is unaudited now. stdaudit: 2798 mapped, 786 gaps, 0 stale, 0
  unmapped; stdcov: features 23 Proven and 67 Solid (bcrypt gains Proven; cbor 76 gaps, metrics 82, uuid 31,
  otp 8, cron 7, jwt 3). - pgx v5.12.0 (pgx, pgconn, pgtype, pgxpool; 1107 symbols), golang.org/x/crypto v0.58.0 argon2, blake2b,
  chacha20poly1305, hkdf, sha3 and pkcs12 (49), golang.org/x/net v0.62.0 http2, h2c and hpack (201, and an h2spec
  checklist item) (2026-11-15): every done outside row is pinned. stdaudit: 3263 mapped, 1401 gaps, 0 stale, 0
  unmapped. The marks do not move (sql, crypto, x509 and http had partial rows already); the gaps are the work
  list. - A row for every std module (rule 5, 2026-11-15): eight Go package rows name their modules (bufio split from io;
  encoding/asn1, net/textproto, html, unicode, math/rand, encoding/xml and sync name std/asn1, std/textproto,
  std/htmlent, std/uniclass, std/random, std/xmlcodec and std/task), nine outside rows pin the library each port
  followed, four modules with no Go reference take `spec/std.md -` pins and checklists. stdaudit -go skips a spec
  pin; a `gopkg.in/x.vN` path keys as `x`. stdaudit: 3423 mapped, 1433 gaps, 0 stale, 0 unmapped; stdcov: features
  32 Proven, 58 Solid. Next: pin the in progress rows (sqlite, timego, protoparse).
- Rule 7 (addendum, Accepted, 2026-11-16): the twins for std/testing, std/amqptest and std/go are the next
  step of task #88 (Xo Lang 2), each in bench/perf with a twin.txt as stdproven perf reads it.
- mattn/go-sqlite3 v1.14.53 (2026-11-16, #109): 138 symbols and an eight item checklist; closing its gaps
  added mattn's other DSN keys, named extended result codes and its time reading (Row.time); 31 rows not
  planned by category (Go callbacks into SQLite and the authorizer, run-time extensions, the deprecated user
  authentication extension); Backup and Serialize map to VACUUM INTO, Deserialize to a file through the Fs.
  stdcov: sqlite features Proven.
