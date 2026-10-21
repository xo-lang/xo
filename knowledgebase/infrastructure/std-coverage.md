# Std coverage against Go's standard library

Decision 0276: Go's standard library is std's reference scope. This
table maps each Go package (go1.26, `go list std` without internal and
vendor packages) to the Xo module that covers it, with a status:

- **done**: the module covers what a backend uses of the package, on
  the native backends and the Go backend.
- **partial**: some of it; the row names what is missing.
- **planned**: nothing yet, wanted (ranked by the survey below).
- **not planned**: one line says why.

"Prelude" means the builtin types and capabilities every module sees
(spec/std.md sections 1 to 3). Spec sections are spec/std.md's.

Last verified: 2026-10-20 (main 4ec9c647; spec/std.md, std/*.xo,
std/native, THIRD_PARTY_NOTICES).

## Modules and change logs

Each module's change log is `std/changes/MODULE.md` (decision 0276 item
6); a module without a file has no entries yet.

| Module | Spec | Change log |
|---|---|---|
| prelude (Str, Bytes, List, Map, Fs, Net, Clock, Rand, Env, Stdio, Proc) | 1 to 3 | `std/changes/prelude.md` |
| std/http | 4 | `std/changes/http.md` |
| std/json | 5 | `std/changes/json.md` |
| std/log | 6 | `std/changes/log.md` |
| std/task | 7 | `std/changes/task.md` |
| std/testing | 8 | `std/changes/testing.md` |
| std/path | 9 | `std/changes/path.md` |
| std/sql | 10 | `std/changes/sql.md` |
| std/crypto | 11 | `std/changes/crypto.md` |
| std/go | 12 | `std/changes/go.md` |
| std/time | 13 | `std/changes/time.md` |
| std/cli | 14 | `std/changes/cli.md` |
| std/term | 15 | `std/changes/term.md` |
| std/csv | 16 | `std/changes/csv.md` |
| std/regexp | 17 | `std/changes/regexp.md` |
| std/archive | 18 | `std/changes/archive.md` |
| std/toml | 19 | `std/changes/toml.md` |
| std/url | 20 | `std/changes/url.md` |
| std/base64, std/base32 | 21 | `std/changes/base64.md`, `std/changes/base32.md` |
| std/binary | 22 | `std/changes/binary.md` |
| std/tls | 27 | `std/changes/tls.md` |
| std/mime | 23 | `std/changes/mime.md` |
| std/uuid | 24 | `std/changes/uuid.md` |
| std/mail | 25 | `std/changes/mail.md` |
| std/multipart | 26 | `std/changes/multipart.md` |
| std/smtp | 29 | `std/changes/smtp.md` |
| std/netip | 28 | `std/changes/netip.md` |

## Go packages

### Text, strings, and encodings

| Go package | Xo | Status | Notes |
|---|---|---|---|
| strings | prelude Str | partial | Has contains, find, split, lines, trim*, replace, repeat, to_lower/upper, join, StrBuilder. Missing: last_index, count, fields, cut, equal_fold, index of a rune or any, split_n, title, Map, Replacer. |
| bytes | prelude Bytes, BytesBuilder | partial | Has len, slicing, `+`, hex, base64, decode_rune, BytesBuilder. Missing: find, split, contains, equal, trim, has_prefix on Bytes; a Reader. |
| strconv | prelude Int.parse, parse_base, to_str_base, Float.parse, Bool.parse, quote, unquote | done | ParseInt, FormatInt, ParseBool, Unquote added (decision 0287). Not planned: the Append functions (StrBuilder covers them). |
| unicode | prelude Rune | partial | is_letter, is_digit, is_space, is_print, to_lower, to_upper (Go's tables, decision 0079). Missing: is_upper, is_lower, is_punct, is_control, range tables by name. |
| unicode/utf8 | prelude Str, Bytes | done | rune_count, decode_rune, UTF-8 validity rules (decision 0259). |
| unicode/utf16 | none | planned | Needed for Windows and JSON surrogate work only inside std today; no user API. |
| encoding/json | std/json | partial | `derive Json`, Patch, typed decode with paths, wire key names (`kind as "type"` and `derive Json(case: ...)`, decision 0277), json.Value for JSON of any shape (2026-10-21). Missing: a streaming decoder, omitempty. |
| encoding/base64 | std/base64, prelude Bytes | done | Std, Url, RawStd, RawUrl with Go's decoding rules (decision 0279). |
| encoding/base32 | std/base32 | done | Std, Hex, RawStd, RawHex (decision 0279). |
| encoding/hex | prelude Bytes | done | to_hex, from_hex, the same as EncodeToString and DecodeString on 1,500 strings (2026-10-20). Dump is not offered. |
| encoding/binary | std/binary | done | Byte orders on Bytes and varints (decision 0279); no Read and Write of structs (no reflection). |
| encoding/csv | std/csv | done | Native port (decision 0164); in memory, no streaming reader. Records match Go's Reader (FieldsPerRecord -1) on 2,022 texts (2026-10-20); error texts and Go's options (Comment, LazyQuotes, TrimLeadingSpace) are Xo's own or missing. |
| encoding/pem | std/crypto | done | pem_decode, pem_encode (decision 0285). |
| encoding/asn1 | inside std/native/llrt/asn1.xo | partial | Used by X.509; no public API. |
| encoding/xml | none | planned | SOAP, SAML, sitemaps, TwiML; lower than JSON gaps. |
| encoding/gob | none | not planned | Go specific wire format; Xo programs exchange JSON. |
| encoding/ascii85 | none | not planned | No backend use (PostScript and PDF). |
| encoding | none | not planned | Interfaces only; Xo uses derive (Json, SqlRow). |
| mime | std/mime | partial | ParseMediaType, FormatMediaType, TypeByExtension and ExtensionsByType (Go's builtin table only, no system files), and net/http's DetectContentType (decision 0282). Missing: encoded words (WordEncoder, WordDecoder, RFC 2047). |
| mime/multipart | std/multipart | partial | Reader (NextPart, ReadForm), Writer, Request.multipart_form, in memory (decision 0282). Missing: streaming parts, multipart/mixed responses. |
| mime/quotedprintable | inside std/native/multipart | partial | The decoder, for quoted-printable parts. Missing: a public module and the encoder (with an SMTP client). |
| html | none | planned | EscapeString for server rendered pages. |
| html/template | none | planned | Contextual escaping templates; after html. |
| text/template | none | planned | Config and mail templates; after html/template. |
| text/tabwriter | none | planned | CLI tables; std/cli and std/term cover most tool output. |
| text/scanner | none | not planned | Go source tokenizer; Xo's own lexer is in selfhost. |
| regexp, regexp/syntax | std/regexp | done | Go's parser, compiler, and Pike VM ported, with every Unicode class of Go's regexp natively (2026-10-21); Go's results on re2-search.txt and on each class. |
| index/suffixarray | none | not planned | Niche; a module can provide it. |
| fmt | string interpolation, Display, Debug | partial | `${x}`, Display, Debug, Float shortest text. Missing: width, padding, precision, and base verbs (`%08.3f`, `%x`, `%-10s`). |

### Containers, sorting, iteration

| Go package | Xo | Status | Notes |
|---|---|---|---|
| sort, slices, maps, cmp | prelude List, Map, Set, Ord | done | sort, sort_by (stable), min, max, clamp, filter, map, fold, keys, values. Binary search is missing. |
| container/heap | none | planned | A priority queue type for schedulers and timers. |
| container/list, container/ring | none | not planned | List and Buf cover them. |
| iter | for loops over List, Map, Set, Range | not planned | No iterator protocol in the language (core). |
| unique, weak, structs | none | not planned | Runtime and layout concerns the language does not expose. |

### Numbers and math

| Go package | Xo | Status | Notes |
|---|---|---|---|
| math | prelude Float | partial | Go's portable algorithms bit for bit on every backend (decision 0287): pow, exp, exp2, expm1, log, log2, log10, log1p, trigonometric and hyperbolic functions, cbrt, hypot, mod, is_inf, to_bits. Missing: constants (Float.PI, Float.INF; a compiler change), erf, gamma, Nextafter, FMA. |
| math/bits | prelude Int word methods | done | rotate, swap_bytes, mul_hi_unsigned, unsigned compare (decision 0260); leading_zeros, trailing_zeros, ones_count (decision 0287). |
| math/big | inside std/native/llrt/bigmod.xo | planned | Needed for RSA and ECDSA keys from JWKs; no public big integer. |
| math/rand, math/rand/v2 | prelude Rand capability | done | Secure in programs, seeded in tests. |
| math/cmplx | none | not planned | No backend use. |
| hash | std/crypto Hasher | partial | A streaming hasher for the cryptographic hashes (decision 0279); no interface user types implement. |
| hash/crc32 | inside std/archive | partial | Used by gzip and zip; no public API. |
| hash/fnv, hash/adler32, hash/crc64, hash/maphash | none | planned | fnv and crc32 public functions are cheap ports; maphash is internal to Map. |

### Cryptography

| Go package | Xo | Status | Notes |
|---|---|---|---|
| crypto/sha256 | std/crypto | done | sha256 and Hash.Sha224, Sha256, with the arm64 SHA2 instructions natively (decision 0231). |
| crypto/hmac | std/crypto hmac, hmac_hasher | done | Over every Hash (decision 0279). |
| crypto/sha1 | std/crypto Hash.Sha1 | done | Decision 0279. |
| crypto/sha512, crypto/sha3 | std/crypto Hash | done | SHA-384, SHA-512, SHA3-256, SHA3-512 (decision 0279); SHAKE is not public. |
| crypto/md5 | std/crypto Hash.Md5 | done | Ported from Go (decision 0279); for protocols that require it. |
| crypto/subtle | std/crypto equal | done | Constant time comparison. |
| crypto/rand | prelude Rand, crypto.random_bytes, token | done | |
| crypto/pbkdf2 | std/crypto hash_password | done | PBKDF2-HMAC-SHA256 in a PHC like string. |
| crypto/ed25519 | std/crypto | done | Decision 0189. |
| crypto/hkdf | inside std/native/llrt/hkdf.xo | partial | TLS only; no public API. |
| crypto/aes, crypto/cipher | inside std/native/llrt (AES-GCM, ChaCha20-Poly1305) | partial | TLS only. Missing: a public AEAD seal and open for encrypted cookies and data at rest. |
| crypto/ecdh | inside std/native/llrt (X25519, nistec) | partial | TLS only. |
| crypto/ecdsa, crypto/rsa, crypto/elliptic | std/crypto | partial | Sign and verify (PKCS #1 v1.5, PSS, ECDSA DER and raw), ECDSA and Ed25519 key generation (decision 0285). Missing: RSA key generation, OAEP encryption, ECDH as an API. |
| crypto/mlkem | inside std/native/llrt/mlkem.xo | partial | TLS key exchange only. |
| crypto/hpke | none | planned | After a public AEAD and ECDH. |
| crypto/x509, crypto/x509/pkix | std/crypto (keys); inside std/native/llrt (x509.xo, x509verify.xo) | partial | Key parsing and marshaling in std/crypto (decision 0285); chain verification for the HTTP client. Missing: a certificate API (parsing, CSRs, generating test certificates). |
| crypto/tls | std/http (with_tls, with_roots, tls_cert), std/tls (client) | partial | TLS 1.2 and 1.3 in Xo (decision 0202); `tls.client` on a dialed Conn (decision 0281: Postgres sslmode, SMTP STARTTLS, Redis). Missing: a server on a Net listener outside HTTP, client certificates. |
| crypto | none | not planned | The Go registry of hash functions; Xo calls functions directly. |
| crypto/des, crypto/dsa, crypto/rc4 | none | not planned | Broken or deprecated algorithms. |
| crypto/fips140 | none | not planned | No FIPS module validation. |

### Networking and HTTP

| Go package | Xo | Status | Notes |
|---|---|---|---|
| net | prelude Net (TCP, Unix sockets), DNS in std/native/llrt/dns.xo | partial | listen, dial, attenuation. Missing: UDP, a name lookup API (LookupHost, SRV, TXT, MX), IP and CIDR types, deadlines on a Conn. |
| net/netip | std/netip | done | Addr, Prefix, ParseAddr, ParsePrefix, Contains, Masked, the Is predicates (decision 0287). Missing: AddrPort, Prefix.Overlaps, Next and Prev. |
| net/url | std/url, std/http parse_url and Url | partial | Escaping (QueryEscape, PathEscape and their inverses) and Values (ParseQuery, Encode) are done (decision 0279). Request.form and query_all are in std/http. Missing: relative references, userinfo, a full URL type outside std/http. |
| net/http (server) | std/http | partial | Router, middleware, mount, JSON, streaming, TLS, HTTP/2. Has Request.form (urlencoded) and query_all (decision 0279). Has repeated header fields (header_values) and cookies (decision 0288), multipart forms (decision 0282). Missing: a streaming request body (whole body in memory, 64 MiB natively), ServeContent and FileServer (Range, If-Modified-Since), response compression, MaxBytesReader per route, timeouts per server. |
| net/http (client) | std/http Client | partial | GET, send, post_json, redirects, gzip, HTTP/2, TLS roots. Missing: cookie jar, proxies, streaming response bodies, per request timeouts. |
| net/http/httptest | std/testing FakeNet, Router.call | done | In process servers and fakes (section 8). |
| net/http/httputil | none | planned | ReverseProxy and DumpRequest. |
| net/http/cookiejar | none | planned | With client cookies. |
| net/http/pprof | runtime profiler (decision 0238, XO_PROF) | partial | CPU and heap profiles as files; no HTTP endpoint. |
| net/http/httptrace | none | not planned | Tracing is the runtime's (decision 0240). |
| net/http/cgi, net/http/fcgi | none | not planned | Legacy deployment models. |
| net/smtp | std/smtp | done | SendMail as `smtp.send` (STARTTLS required by default, PLAIN, CRAM-MD5), decision 0290. Not planned: the step by step Client. |
| net/mail | std/mail | partial | ParseAddress, ParseAddressList, Address.String with RFC 2047 words (decision 0282). Missing: ReadMessage, headers, ParseDate. |
| net/textproto | inside std/native/llrt (http1) | partial | Header canonicalization for HTTP only. |
| net/rpc, net/rpc/jsonrpc | none | not planned | Superseded by HTTP JSON APIs. |

### Files, processes, and the OS

| Go package | Xo | Status | Notes |
|---|---|---|---|
| os | prelude Fs, Env, Proc, Stdio | done | Capability based (sections 3.1 to 3.7). File has no seek or read_at. |
| os/exec | prelude Proc, Cmd, Child | done | Section 3.8. |
| os/signal | core 7.5 shutdown, task.stop_requested, Proc.kill | done | Two phase stop on SIGTERM and SIGINT. |
| os/user | Env.home | partial | Home directory only. |
| io, io/fs, bufio, io/ioutil | File, Conn, Child read and write, Fs.walk | partial | Byte streams exist per handle. Missing: a common reader and writer interface, buffered readers over a Conn. |
| path | std/path | done | |
| path/filepath | prelude Fs paths, std/path | done | Every Fs path uses `/` on every OS (section 3.1). |
| embed | none | planned | Embedding files (migrations, templates, static assets) in a program. |
| syscall | none | not planned | Capabilities replace raw system calls. |
| log | std/log | done | |
| log/slog | std/log fields, Format.Json | partial | Structured fields and JSON. Missing: span and request id (marked later in section 6), handlers. |
| log/syslog | none | not planned | Services log to standard error; the platform collects it. |
| expvar | none | planned | Metrics endpoint; a Prometheus text helper fits better. |
| flag | std/cli | done | Decision 0160. |
| time | prelude Time, Duration, std/time | done | Zones, civil time, named layouts (decision 0094). |
| time/tzdata | std/tzdata, embedded per use | done | |
| context | scopes, cancellation, task.with_timeout | done | Structured concurrency (core 7). |
| sync, sync/atomic | prelude Mutex, Atomic, Chan, task.once, semaphore | done | |
| errors | `!` results, Error interface | done | Typed errors (core 2.6). |

### Data and compression

| Go package | Xo | Status | Notes |
|---|---|---|---|
| database/sql | std/sql | partial | API, pool limits, transactions, derive SqlRow, an in memory driver; Blob, JsonText, and Array values and SQLSTATE codes (decision 0278 stage A). A Postgres driver written in Xo, `sql.open(net, "postgres", dsn)` with sslmode through std/tls, on every native target (decision 0278). Missing: other network drivers (MySQL), prepared statements, row streaming. |
| database/sql/driver | none | planned | A driver interface in Xo so drivers are std/sql modules (Postgres first). |
| compress/gzip, compress/flate | std/archive gzip, gunzip | done | In memory. Missing: streaming. |
| compress/zlib | none | planned | Small on top of flate. |
| compress/bzip2, compress/lzw | none | not planned | Read only legacy formats. |
| archive/tar, archive/zip | std/archive | done | Go's results natively on Go's small test archives (zip64 and PAX sparse files included) and on written archives (2026-10-21). |

### Testing

| Go package | Xo | Status | Notes |
|---|---|---|---|
| testing | test blocks, std/testing | done | Property tests, fakes, captured logs (core 9, section 8). |
| testing/quick | property tests | done | |
| testing/fstest | MemFs | done | |
| testing/synctest | FakeClock, deterministic scheduling under xo test | done | |
| testing/iotest, testing/slogtest, testing/cryptotest | none | not planned | Go interface conformance kits. |

### Not planned: toolchain, runtime, and images

| Go package | Status | Why |
|---|---|---|
| go/* (ast, build, constant, doc, format, importer, parser, printer, scanner, token, types, version) | not planned | Go source tooling; Xo's compiler is in selfhost/. |
| debug/* (buildinfo, dwarf, elf, gosym, macho, pe, plan9obj) | not planned | Binary formats are the toolchain's (selfhost/macho and the linkers), not a backend need. |
| runtime, runtime/debug, runtime/metrics, runtime/pprof, runtime/trace, runtime/race, runtime/coverage, runtime/cgo | not planned | The runtime is Xo's own; profiling (0238), tracing (0240), race (0273) are toolchain features. |
| plugin | not planned | Static binaries only. |
| reflect, unsafe | not planned | No reflection; derive replaces it. |
| image, image/* | not planned | Image work is not std's backend scope; a module can port it. |

## Outside Go's standard library

| Need | Go source | Xo | Status | Notes |
|---|---|---|---|---|
| Postgres driver | github.com/jackc/pgx, lib/pq | none | planned | Pure Xo wire protocol (v3, SCRAM-SHA-256, TLS) over Net, as a std/sql driver. |
| MySQL driver | go-sql-driver/mysql | none | planned | After Postgres. |
| SQLite | modernc.org/sqlite, mattn/go-sqlite3 | none | planned | Through `use c` or a port; after Postgres. |
| Redis client | redis/go-redis | none | planned | RESP over Net. |
| Argon2, BLAKE2b | golang.org/x/crypto | std/crypto | done | Decision 0059. |
| ChaCha20-Poly1305, HKDF, SHA-3 | golang.org/x/crypto | inside std/native/llrt | partial | TLS only, no API. |
| bcrypt | golang.org/x/crypto/bcrypt | none | planned | To verify hashes migrated from other systems. |
| WebSocket | golang.org/x/net/websocket, gorilla, coder/websocket | none | planned | Needs connection hijacking in std/http. |
| HTTP/2 | golang.org/x/net/http2 | std/http | done | Decision 0121. |
| JWT, JOSE | golang-jwt, go-jose | none | planned | HS256 is writable today by hand; RS256 and ES256 need public RSA and ECDSA. |
| UUID | google/uuid | std/uuid | done | v4 and v7 from Rand and Clock, parsing (decision 0282). |
| OAuth2 client | golang.org/x/oauth2 | none | planned | Needs form encoding. |
| TOTP | pquerna/otp | none | planned | Needs HMAC-SHA1 and base32. |
| Migrations | golang-migrate, goose | none | planned | Needs embed. |
| Cron schedules | robfig/cron | none | planned | |
| Prometheus exposition | prometheus/client_golang | none | planned | |
| S3 and AWS SigV4 | aws-sdk-go-v2 | none | not planned | A third party module; std provides HMAC and SHA-256. |
| gRPC, protobuf | google.golang.org/grpc | none | not planned | Large; a module, after HTTP/2 trailers are exposed. |

## Survey of backend gaps (2026-10-20)

Five services written from spec/std.md alone (a CRUD API on Postgres, a
webhook receiver for GitHub, Stripe, and Twilio, a file upload service,
a job worker, and an auth service with JWT, TOTP, and OAuth2), type
checked with `xo check`, each missing piece marked where the code needed
it. Ranked by how many of the five hit the gap:

| Rank | Gap | Services | Go reference |
|---|---|---|---|
| 1 | Postgres driver (natively `sql.open` has no driver at all) | 5 of 5 | pgx, database/sql/driver |
| 2 | SqlValue types: bytes, JSON, arrays; driver error codes | 3 of 5 | database/sql, pgx |
| 3 | URL query and form encoding, escaping, repeated values | 3 of 5 | net/url |
| 4 | Base64 URL and raw encodings, base32, binary | 2 of 5 | encoding/base64, base32, binary |
| 5 | SHA-1, HMAC over other hashes, streaming hash | 2 of 5 | crypto/sha1, crypto/hmac, hash |
| 6 | Repeated header fields and cookies | 2 of 5 | net/http Header, Cookie |
| 7 | TLS on a Conn, SMTP, mail addresses | 2 of 5 | crypto/tls, net/smtp, net/mail |
| 8 | JSON key names (keywords, camelCase) | 1 of 5 (most third party payloads) | encoding/json struct tags |
| 9 | Multipart, MIME types, content sniffing, ServeContent, streaming request bodies | 1 of 5 | mime, mime/multipart, net/http |
| 10 | Public RSA and ECDSA verify, big integers | 1 of 5 | crypto/rsa, crypto/ecdsa, math/big |
| 11 | UUID | 1 of 5 | google/uuid |
| 12 | Int to text in other bases, Float math, embed, XML, client IP parsing | 1 of 5 each | strconv, math, embed, encoding/xml, net/netip |
