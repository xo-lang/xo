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

Last verified: 2026-10-29 (main c21a17a84, and main edb47f207 for the
html, html/template, crypto/aes, crypto/hkdf, x/crypto rows and survey
gaps 4, 5, 15 and 16; spec/std.md, `xo doc` of the
prelude types, `xo symbols --module=std/... --pub` of every module, the
partial and planned rows rechecked; done rows from 2026-10-20 stand).

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
| std/template | 30 | `std/changes/template.md` |
| std/embed | 31 | `std/changes/embed.md` |
| std/compress | 32 | `std/changes/compress.md` |
| std/big | 33 | `std/changes/big.md` |
| std/xml | 34 | `std/changes/xml.md`, `std/changes/xmlcodec.md` |
| std/websocket | 35 | `std/changes/websocket.md` |
| std/debug | 36 | `std/changes/debug.md` |
| std/metrics | 37 | `std/changes/metrics.md` |
| std/trace | 38 | `std/changes/trace.md` |
| std/html | 39 | `std/changes/html.md` |
| std/tabwriter | 40 | `std/changes/tabwriter.md` |
| std/x509 | 41 | `std/changes/x509.md` |
| std/rate | 42 | `std/changes/rate.md` |
| std/backoff | 43 | `std/changes/backoff.md` |

## Go packages

### Text, strings, and encodings

| Go package | Xo | Status | Notes |
|---|---|---|---|
| strings | prelude Str | done | contains, find, rfind, count, fields, split, split_n, split_after(_n), cut, cut_prefix, cut_suffix, equal_fold, find_any, rfind_any, contains_any, contains_rune, the `_where` forms (IndexFunc, FieldsFunc, TrimFunc...), trim_chars (Trim, TrimLeft, TrimRight), replace_n, map_runes (Map), repeat, lines, to_lower/upper, join, StrBuilder (2026-10-29). Not offered: Replacer (one pass over several replacements), Title (deprecated in Go). |
| bytes | prelude Bytes, BytesBuilder | done | Str's methods on Bytes: find, rfind, count, contains (any, rune, where), split (n, after), cut, fields, trim (prefix, suffix, where), replace(_n), to_lower/upper, equal_fold, starts_with, ends_with; `==` is Equal; hex, base64, decode_rune, zeroed; BytesBuilder with put_le and put_be (decision 0399) (2026-10-29). Missing: a Reader (streams are per handle). |
| strconv | prelude Int.parse, parse_base, to_str_base, Float.parse, Bool.parse, quote, unquote | done | ParseInt, FormatInt, ParseBool, Unquote added (decision 0287). Not planned: the Append functions (StrBuilder covers them). |
| unicode | prelude Rune | done | is_letter, is_digit, is_number, is_space, is_upper, is_lower, is_title, is_punct, is_control, is_graphic, is_mark, is_symbol, is_print, to_lower, to_upper, to_title (Go's tables, decision 0079) (2026-10-29). Not offered: range tables by name (unicode.In, unicode.Han). |
| unicode/utf8 | prelude Str, Bytes | done | rune_count, decode_rune, UTF-8 validity rules (decision 0259). |
| unicode/utf16 | none | planned | Needed for Windows and JSON surrogate work only inside std today; no user API. |
| encoding/json | std/json | partial | `derive Json`, Patch, typed decode with paths, wire key names (`kind as "type"` and `derive Json(case: ...)`, decision 0277), json.Value for JSON of any shape (2026-10-21), `as "key,omitempty"` and the push-style json.Decoder of value streams (decision 0309). Missing: json.Value as a field of a `derive Json` struct or a `post_json` argument (RawMessage, `any`), indented output (MarshalIndent), Number. |
| encoding/base64 | std/base64, prelude Bytes | done | Std, Url, RawStd, RawUrl with Go's decoding rules (decision 0279). |
| encoding/base32 | std/base32 | done | Std, Hex, RawStd, RawHex (decision 0279). |
| encoding/hex | prelude Bytes | done | to_hex, from_hex, the same as EncodeToString and DecodeString on 1,500 strings (2026-10-20). Dump is not offered. |
| encoding/binary | std/binary | done | Byte orders on Bytes and varints (decision 0279); no Read and Write of structs (no reflection). |
| encoding/csv | std/csv | done | Native port (decision 0164); in memory, no streaming reader. Records match Go's Reader (FieldsPerRecord -1) on 2,022 texts (2026-10-20); error texts and Go's options (Comment, LazyQuotes, TrimLeadingSpace) are Xo's own or missing. |
| encoding/pem | std/crypto | done | pem_decode, pem_encode (decision 0285). |
| encoding/asn1 | inside std/native/llrt/asn1.xo | partial | Used by X.509; no public API. |
| encoding/xml | std/xml | partial | Go's Decoder tokens (strict, HTML-lenient, raw), an element tree, and the Encoder, on every backend (decision 0314, 2026-10-21). Missing: Marshal and Unmarshal of structs (stage 2), streaming. |
| encoding/gob | none | not planned | Go specific wire format; Xo programs exchange JSON. |
| encoding/ascii85 | none | not planned | No backend use (PostScript and PDF). |
| encoding | none | not planned | Interfaces only; Xo uses derive (Json, SqlRow). |
| mime | std/mime | partial | ParseMediaType, FormatMediaType, TypeByExtension and ExtensionsByType (Go's builtin table only, no system files), and net/http's DetectContentType (decision 0282). Missing: encoded words (WordEncoder, WordDecoder, RFC 2047). |
| mime/multipart | std/multipart | partial | Reader (NextPart, ReadForm), Writer, Request.multipart_form, in memory (decision 0282). Missing: streaming parts, multipart/mixed responses. |
| mime/quotedprintable | inside std/native/multipart and mail | partial | The decoder (multipart parts) and the encoder (mail.compose, decision 0290). Missing: a public module. |
| html | std/html | done | escape_string and unescape_string, Go's html on its escape_test.go cases, the entity table generated from Go's entity.go (decision 0408, 2026-10-29). |
| html/template | std/template | partial | `template.parse_html`: Go's contextual autoescaping, trusted content as `json.Value.Trusted` (`template.html`, `attr`; Go's template.HTML, HTMLAttr), template functions (decision 0408, 2026-10-29). Missing what text/template's row lists. |
| text/template | std/template | done | Go's text/template over json.Value data (decision 0299). Template functions (`funcs: Map[Str, TemplateFn]`, Go's FuncMap; decision 0408). Not offered: Clone and Lookup, ParseFS. |
| text/tabwriter | std/tabwriter (decision 0410) | done | Elastic tabstops with Go's flags and escapes; the native backends run an Xo port of Go's code and tests, the Go backend Go's own Writer; `flush` returns the text. |
| text/scanner | none | not planned | Go source tokenizer; Xo's own lexer is in selfhost. |
| regexp, regexp/syntax | std/regexp | done | Go's parser, compiler, and Pike VM ported, with every Unicode class of Go's regexp natively (2026-10-21); Go's results on re2-search.txt and on each class. |
| index/suffixarray | none | not planned | Niche; a module can provide it. |
| fmt | string interpolation, Display, Debug | partial | `${x}`, Display, Debug, Float shortest text. Missing: width, padding, precision, and base verbs (`%08.3f`, `%x`, `%-10s`). |

### Containers, sorting, iteration

| Go package | Xo | Status | Notes |
|---|---|---|---|
| sort, slices, maps, cmp | prelude List, Map, Set, Ord | done | sort, sort_by (stable), sort_in_place, group_by, filter, map, fold, find, index_of, keys, values; min, max, clamp on any Ord value. Missing: binary search, List min and max (slices.Min), dedup (slices.Compact), chunks. |
| container/heap | none | planned | A priority queue type for schedulers and timers. |
| container/list, container/ring | none | not planned | List and Buf cover them. |
| iter | for loops over List, Map, Set, Range | not planned | No iterator protocol in the language (core). |
| unique, weak, structs | none | not planned | Runtime and layout concerns the language does not expose. |

### Numbers and math

| Go package | Xo | Status | Notes |
|---|---|---|---|
| math | prelude Float | partial | Go's portable algorithms bit for bit on every backend (decision 0287): pow, exp, exp2, expm1, log, log2, log10, log1p, trigonometric and hyperbolic functions, cbrt, hypot, fma, mod, is_inf, to_bits, from_bits. Missing: constants (Float.PI, Float.INF; a compiler change), erf, gamma, Nextafter. |
| math/bits | prelude Int word methods | done | rotate, swap_bytes, mul_hi_unsigned, unsigned compare (decision 0260); leading_zeros, trailing_zeros, ones_count (decision 0287). |
| math/big | std/big | partial | big.BigInt (Go's Int, immutable; decision 0311, 2026-10-21) with Go's results on every backend; natively schoolbook (no Karatsuba or recursive division yet). Missing: Rat, Float, Json. |
| math/rand, math/rand/v2 | prelude Rand capability | done | Secure in programs, seeded in tests. |
| math/cmplx | none | not planned | No backend use. |
| hash | std/crypto Hasher | partial | A streaming hasher for the cryptographic hashes (decision 0279); no interface user types implement. |
| hash/crc32 | std/crc32 | done | IEEE and Castagnoli, `checksum` and `update` (spec/std.md 44); Castagnoli in software (Go uses CRC32C instructions). |
| hash/fnv | std/fnv | done | 32, 64 and 128 bit FNV-1 and FNV-1a, `sum*` and `update*` (spec/std.md 45). |
| hash/adler32, hash/crc64, hash/maphash | none | planned | maphash is internal to Map. |

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
| crypto/hkdf | std/crypto | done | `hkdf_extract`, `hkdf_expand`, `hkdf` over every Hash (Go's Extract, Expand, Key), on TLS's llrt code (2026-10-29, `std/changes/crypto.md`). |
| crypto/aes, crypto/cipher | std/crypto | partial | AEAD: `aead_key(Aead.AesGcm or ChaCha20Poly1305, key)`, `seal`/`open` with a nonce and additional data, `seal_random`/`open_random` (Go's NewGCMWithRandomNonce) (2026-10-29). Missing: the raw block cipher and the unauthenticated modes (CBC, CTR, CFB, OFB). |
| crypto/ecdh | inside std/native/llrt (X25519, nistec) | partial | TLS only. |
| crypto/ecdsa, crypto/rsa, crypto/elliptic | std/crypto | partial | Sign and verify (PKCS #1 v1.5, PSS, ECDSA DER and raw), ECDSA and Ed25519 key generation (decision 0285). Missing: RSA key generation, OAEP encryption, ECDH as an API. |
| crypto/mlkem | inside std/native/llrt/mlkem.xo | partial | TLS key exchange only. |
| crypto/hpke | none | planned | After a public AEAD and ECDH. |
| crypto/x509, crypto/x509/pkix | std/crypto (keys); inside std/native/llrt (x509.xo, x509verify.xo) | partial | Key parsing and marshaling in std/crypto (decision 0285); chain verification for the HTTP client. Missing: a certificate API (parsing: NotAfter, subject, SANs; CSRs; generating test certificates) and a TLS connection's peer certificates (ConnectionState). |
| crypto/tls | std/http (with_tls, with_roots, tls_cert), std/tls (client) | partial | TLS 1.2 and 1.3 in Xo (decision 0202); `tls.client` on a dialed Conn (decision 0281: Postgres sslmode, SMTP STARTTLS, Redis). Missing: a server on a Net listener outside HTTP, client certificates. |
| crypto | none | not planned | The Go registry of hash functions; Xo calls functions directly. |
| crypto/des, crypto/dsa, crypto/rc4 | none | not planned | Broken or deprecated algorithms. |
| crypto/fips140 | none | not planned | No FIPS module validation. |

### Networking and HTTP

| Go package | Xo | Status | Notes |
|---|---|---|---|
| net | prelude Net (TCP, Unix sockets, lookup_host, dial_timeout), std/udp, std/dns (MX, TXT, SRV, PTR), DNS client in std/native/llrt/dns.xo | partial | listen, dial, attenuation, UDP, record lookups. Missing: LookupCNAME and LookupNS, JoinHostPort and SplitHostPort, network interfaces, IP and CIDR types outside std/netip, deadlines on a Conn; native DNS record lookups on Windows. |
| net/netip | std/netip | done | Addr, Prefix, ParseAddr, ParsePrefix, Contains, Masked, the Is predicates (decision 0287). Missing: AddrPort, Prefix.Overlaps, Next and Prev. |
| net/url | std/url, std/http parse_url and Url | partial | Escaping (QueryEscape, PathEscape and their inverses) and Values (ParseQuery, Encode) are done (decision 0279). Request.form and query_all are in std/http. Missing: relative references, userinfo, a full URL type outside std/http. |
| net/http (server) | std/http | partial | Router, middleware, mount, JSON, streaming responses, TLS, HTTP/2, gzip_responses (decision 0305), Request.form and query_all (decision 0279), repeated header fields and cookies (decision 0288), multipart forms (decision 0282), streamed request bodies (`handle_stream`, BodyReader, decision 0310), `serve_content` (ServeContent: Range, conditional requests), client_ip behind proxies (2026-10-29). Missing: FileServer over an Fs directory, MaxBytesReader per route, read and idle timeouts per server. |
| net/http (client) | std/http Client | partial | get, send, post_json, redirects, gzip, HTTP/2, TLS roots, one timeout per client. Missing: cookie jar, proxies, streaming response bodies (Response.body is whole), a timeout per request. |
| net/http/httptest | std/testing FakeNet, Router.call | done | In process servers and fakes (section 8). |
| net/http/httputil | none | planned | ReverseProxy and DumpRequest. |
| net/http/cookiejar | none | planned | With client cookies. |
| net/http/pprof | std/debug router (decision 0325) | done | CPU and heap profiles over HTTP under a prefix the program mounts, behind its auth function; XO_PROF for files (decision 0238). |
| net/http/httptrace | none | not planned | Tracing is the runtime's (decision 0240). |
| net/http/cgi, net/http/fcgi | none | not planned | Legacy deployment models. |
| net/smtp | std/smtp | done | SendMail as `smtp.send` (STARTTLS required by default, PLAIN, CRAM-MD5), decision 0290. Not planned: the step by step Client. |
| net/mail | std/mail | done | ParseAddress, ParseAddressList, Address.String (decision 0282); ReadMessage, Header.Get, ParseDate, and a message builder, compose (decision 0290). Not planned: AddressParser with a custom charset reader. |
| net/textproto | inside std/native/llrt (http1) | partial | Header canonicalization for HTTP only. |
| net/rpc, net/rpc/jsonrpc | none | not planned | Superseded by HTTP JSON APIs. |

### Files, processes, and the OS

| Go package | Xo | Status | Notes |
|---|---|---|---|
| os | prelude Fs, Env, Proc, Stdio | partial | Capability based (sections 3.1 to 3.7). Missing (survey 2026-10-29): Hostname, Getpid, process rusage, statfs (disk size and free space); File has no seek or read_at. |
| os/exec | prelude Proc, Cmd, Child | done | Section 3.8. |
| os/signal | core 7.5 shutdown, task.stop_requested, Proc.kill | partial | Two phase stop on SIGTERM and SIGINT. Missing: subscribing to other signals (SIGHUP for a reload, SIGUSR1), signal.Notify (2 of 5 services in the 2026-10-29 survey). |
| os/user | Env.home | partial | Home directory only. |
| io, io/fs, bufio, io/ioutil | File, Conn, Child read and write, Fs.walk | partial | Byte streams exist per handle. Missing: a common reader and writer interface, buffered readers over a Conn. |
| path | std/path | done | Clean, Join, Dir, Base, Ext, Split and IsAbs, and Go's Match as `is_match` (ErrBadPattern as `PathErr`, 2026-10-23). |
| path/filepath | prelude Fs paths, std/path | done | Every Fs path uses `/` on every OS (section 3.1). |
| embed | std/embed (with core 3.1's `embed`) | done | a constant of `Str`, `Bytes`, or `embed.Dir` set to `embed("path or glob")`, decision 0304: one file as text or bytes, or a tree; `Dir.fs()` is a read-only `Fs` over it (decision 0307), as embed.FS is an fs.FS. Not covered: `open` as a stream over an embedded file (read the whole file). |
| syscall | none | not planned | Capabilities replace raw system calls. |
| log | std/log | done | |
| log/slog | std/log fields, Format.Json, std/trace | partial | Structured fields and JSON; request ids and a ring of spans in std/trace (decision 0325). Missing: handlers (a sink per level or destination), attributes carried by a logger value. |
| log/syslog | none | not planned | Services log to standard error; the platform collects it. |
| expvar | std/metrics | done | Counters, gauges, histograms with labels in a Registry, its Prometheus text and handler (decision 0325, section 37). expvar's JSON form is not offered. |
| flag | std/cli | done | Decision 0160. |
| time | prelude Time, Duration, std/time | done | Zones, civil time, named layouts (decision 0094). |
| time/tzdata | std/tzdata, embedded per use | done | |
| context | scopes, cancellation, task.with_timeout | done | Structured concurrency (core 7). |
| sync, sync/atomic | prelude Mutex, Atomic, Chan, task.once, semaphore | done | |
| errors | `!` results, Error interface | done | Typed errors (core 2.6). |

### Data and compression

| Go package | Xo | Status | Notes |
|---|---|---|---|
| database/sql | std/sql | partial | API, pool limits, transactions, derive SqlRow, an in memory driver; Blob, JsonText, and Array values and SQLSTATE codes (decision 0278 stage A); the Postgres driver written in Xo, `sql.open(net, "postgres", dsn)` with sslmode through std/tls (decision 0278); a public Driver interface and `open_driver`. Missing: prepared statements, row streaming (a query returns every row), other network drivers (MySQL). |
| database/sql/driver | std/sql Driver, DriverConn, DriverRows, open_driver | done | A driver is an Xo value passed to `sql.open_driver` (2026-10-29). |
| compress/gzip, compress/flate | std/compress (and std/archive gzip, gunzip) | done | Every level, Go's bytes and errors, max_size, streaming Compressor and Decompressor (decision 0305, 2026-10-21). http.gzip_responses next. |
| compress/zlib | std/compress zlib, unzlib | done | Go's writer and reader (decision 0305, 2026-10-21). |
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
| Postgres driver | github.com/jackc/pgx, lib/pq | std/sql | done | Pure Xo wire protocol (v3, SCRAM-SHA-256, TLS) over Net (decision 0278). |
| MySQL driver | go-sql-driver/mysql | none | planned | After Postgres. |
| SQLite | modernc.org/sqlite, mattn/go-sqlite3 | none | planned | Through `use c` or a port; after Postgres. |
| Redis client | redis/go-redis | none | planned | RESP over Net. |
| Argon2, BLAKE2b | golang.org/x/crypto | std/crypto | done | Decision 0059. |
| ChaCha20-Poly1305, HKDF, SHA-3 | golang.org/x/crypto | std/crypto | done | ChaCha20-Poly1305 through `aead_key`, HKDF (`hkdf`), SHA3-256 and SHA3-512 as `Hash` (2026-10-29). |
| bcrypt | golang.org/x/crypto/bcrypt | none | planned | To verify hashes migrated from other systems. |
| WebSocket | golang.org/x/net/websocket, gorilla, coder/websocket | std/websocket | partial | gorilla's behavior, ws:// both sides and wss:// in dial (decision 0313). Missing: permessage-deflate, a message written in pieces, the Autobahn run (stage 3). |
| HTTP/2 | golang.org/x/net/http2 | std/http | done | Decision 0121. |
| JWT, JOSE | golang-jwt, go-jose | none | planned | HS256 is writable today by hand; RS256 and ES256 need public RSA and ECDSA. |
| UUID | google/uuid | std/uuid | done | v4 and v7 from Rand and Clock, parsing (decision 0282). |
| OAuth2 client | golang.org/x/oauth2 | none | planned | Needs form encoding. |
| TOTP | pquerna/otp | none | planned | Needs HMAC-SHA1 and base32. |
| Migrations | golang-migrate, goose | none | planned | std/embed landed (28b3fe3b), so migration files can be embedded. |
| Cron schedules | robfig/cron | none | planned | |
| Prometheus exposition | prometheus/client_golang | std/metrics | done | Registry, counters, gauges, histograms, text exposition and handler (section 37). |
| S3 and AWS SigV4 | aws-sdk-go-v2 | none | not planned | A third party module; std provides HMAC and SHA-256. |
| gRPC, protobuf | google.golang.org/grpc | none | not planned | Large; a module, after HTTP/2 trailers are exposed. |

## Survey of backend gaps (2026-10-30)

Five more services, of kinds the two earlier surveys did not cover,
written from spec/core.md and spec/std.md alone (std's sources unread)
and type checked with `xo check` to 0 errors, each missing piece marked
`// GAP:` where the code needed it (bench/survey-2026-10-30): a
WebSocket chat and notification hub with presence (chathub, 600 lines),
a reliable queue worker speaking the Redis wire protocol (queueworker,
713), a gRPC service and client over HTTP/2 with hand written protobuf
(grpcsvc, 686), a double entry money ledger on Postgres (ledger, 607),
and a CSV, TSV and XLSX import and export batch job (dataport, 537).
Channels, `select`, `task.stopping`, `with_timeout`, std/metrics, rate,
backoff, uuid, HMAC, `json.Value`, websocket upgrade, BigInt, xml and
archive covered most of what they needed. Ranked blockers first, then by
how many services hit the gap:

| Rank | Gap | Services | Blocking | Go reference | Size |
|---|---|---|---|---|---|
| 1 | HTTP/2 trailers: a handler cannot send trailers after the body, the client cannot read them; and no full duplex stream (a `handle_stream` body cannot be read while `http.stream` writes) | grpcsvc (every gRPC server and client) | blocker | net/http ResponseWriter.Trailer, TrailerPrefix, Response.Trailer, ResponseController.EnableFullDuplex | M to L |
| 2 | Mutual TLS: a server cannot request and verify client certificates, a client cannot present one | grpcsvc (service meshes, internal APIs) | blocker | crypto/tls Config ClientAuth, ClientCAs, Certificates | M |
| 3 | A streaming CSV reader over a File (csv.parse takes the whole text; read_line breaks quoted newlines) | dataport | blocker for large files | encoding/csv NewReader(io.Reader).Read | S to M |
| 4 | Text encodings: UTF-16 (Excel's Unicode Text) and Windows-1252 decoding and encoding | dataport | blocker | x/text/encoding/unicode, charmap | S to M |
| 5 | Deadlines on Conn, TlsConn and WsConn (read and write), a WebSocket pong handler; and reading the current task's deadline to pass on (grpc-timeout) | queueworker, chathub, grpcsvc | awkward (with_timeout cancels, and the connection is then lost) | net.Conn SetDeadline, gorilla SetWriteDeadline, SetPongHandler, context.Deadline | M |
| 6 | An exact decimal type (parse, format, rounding modes, allocation) | ledger, dataport | awkward (hand written on BigInt or cents) | shopspring/decimal, cockroachdb/apd (outside Go's std) | M |
| 7 | A type's own JSON form (Money as "12.34") and internally tagged enums (`{"type": "say", ...}`) | ledger, chathub | awkward | encoding/json Marshaler, Unmarshaler | M |
| 8 | A buffered reader over a Conn or Stream (read_line, read_exact, peek) | queueworker (every line protocol) | awkward | bufio.Reader | S |
| 9 | Large data paths: a row cursor for queries (results are read whole), COPY FROM, a zip reader and writer by entry, a streaming XML token reader | dataport | awkward | database/sql Rows, pgx CopyFrom, archive/zip, encoding/xml Decoder | M each |
| 10 | HTTP/2 without TLS (h2c, prior knowledge) and server HTTP/2 settings (max streams, keepalive ping, GOAWAY by age) | grpcsvc | awkward | x/net/http2 AllowHTTP, http.Protocols, http2.Server | M |
| 11 | Protobuf runtime and a gRPC library (`derive Proto` is reserved for after 1.0) | grpcsvc | awkward (hand written) | google.golang.org/protobuf, grpc (outside Go's std) | L |
| 12 | Unicode normalization (NFC) and IDNA | dataport | awkward | x/text/unicode/norm, x/net/idna | M |
| 13 | Minor: `Db.begin` isolation options; Duration round and truncate; lossy Bytes to Str (read_text is lossy, gunzipped bytes are not); identifier quoting for SQL; two digit years; trace ids on outbound requests; Redis and JWT clients (outside Go's std too) | ledger, dataport, chathub, queueworker | minor | sql.TxOptions, Duration.Round, strings.ToValidUTF8, pgx.Identifier, otelhttp | S each |

Language obstacles the services met (for the language owners): enum
variants cannot carry explicit numbers (gRPC codes became Int
constants); no bit preserving Int to UInt64 conversion (`UInt64(x)`
faults on a negative int64 wire value); no cursor to consume a `Bytes`
from the front (each fill copies the unread rest); a `Str` cannot be
indexed by byte; a struct of handles is copied into every spawn
(XO0534, 44 and 58 heap fields) with no way to share it; no
`List.filter_map` and no deque; `backoff.retry` has no notify hook (Go's
RetryNotify) and its `retryable` must be pure; no 128 bit integer.

Found wrong or missing in the spec: std 2.4 shows no way to write an
empty `Set` (no `Set.new`, no literal; `[].to_set()` works); std 33
lists BigInt's methods without result types (`quo_rem`, `cmp`, `sign`);
core 2.10 says branches are not converted when joined, yet `if tls {
tls_conn } else { conn }` checks as a `Stream` (queueworker:244); core
7.3 does not give a select arm's bound type (`T?`, None when the channel
closed); std 4.4 does not say whether an upgraded WebSocket counts as an
in flight request at a stop; std 37's `reg.middleware(clock, label)` is
only in the prose; std 2.6 lists `Time.UNIX_EPOCH` as a value and
Appendix A as a function. Fixed 2026-10-30: six in the spec (e5fe92b08);
the WebSocket at a stop was a backend difference (the Go backend did not
wait), decided in 0429 (code after the seed).

Proposed (blockers that unblock a class of service first): 5 (deadlines,
three services), 1 and 2 (gRPC and mTLS, one design), 3 and 8 (streaming
CSV over a buffered reader), 4, 7, then 6 (decimal, likely outside std
as in Go).

## Survey of backend gaps (2026-10-29)

Five services written from spec/std.md and spec/core.md alone, without
reading std's sources, and type checked with `xo check` (each 0 errors),
each missing piece marked where the code needed it: an API gateway with
a reverse proxy (468 lines), a server rendered web app with templates,
encrypted cookie sessions and CSRF (575), an ops CLI with tables and a
concurrent hashing walk (719), a job scheduler with priorities, cron and
retries on Postgres (726), and a node metrics exporter and sidecar with
statsd, DNS discovery and certificate expiry checks (583). Ranked by how
many services hit the gap, blockers (no workaround) first:

| Rank | Gap | Services | Blocking | Go reference | Size |
|---|---|---|---|---|---|
| 1 | HTTP client: streamed response and request bodies, a redirect policy, a timeout per request, whether a failed request reached the server; then a reverse proxy | gateway (and every proxy or large download) | blocker | net/http Response.Body, Request.Body, CheckRedirect, httputil.ReverseProxy | M |
| 2 | Signals other than SIGINT and SIGTERM (SIGHUP reload) | gateway, exporter | blocker | os/signal Notify | S |
| 3 | json.Value as a Json field or argument (RawMessage, `any`); indented output | scheduler, CLI | blocker | encoding/json RawMessage, MarshalIndent | S |
| 4 | Closed 2026-10-29 (std/crypto aead_key, hkdf). Public AEAD (AES-GCM, ChaCha20-Poly1305) and HKDF, for encrypted cookies and data at rest (both exist inside TLS) | web app | blocker | crypto/cipher, crypto/aes, crypto/hkdf, x/crypto/chacha20poly1305 | S |
| 5 | Closed 2026-10-29 (std/html, template.parse_html, decision 0408). HTML escaping and contextual auto escaping templates, a trusted HTML type, template functions | web app | blocker (XSS by default) | html, html/template, template.FuncMap | L |
| 6 | Closed 2026-10-29 (std/udp, Net.lookup_host, Net.dial_timeout, decision 0406; std/dns lookup_mx, lookup_txt, lookup_srv, lookup_addr, decision 0417). UDP sockets and a DNS lookup API (host, SRV), a dial timeout | exporter | blocker | net.ListenPacket, net.LookupSRV, net.DialTimeout | M |
| 7 | X.509 certificate parsing and a TLS connection's peer certificates | exporter | blocker | crypto/x509 ParseCertificate, tls.ConnectionState | M (the parser exists inside TLS) |
| 8 | Cron schedules | scheduler | blocker | robfig/cron (outside Go's std) | M |
| 9 | Host facts: Hostname, Getpid, rusage, statfs, network interfaces | exporter, scheduler | blocker (statfs), awkward | os, syscall.Statfs, net.Interfaces | S each |
| 10 | std/metrics: gauges and collectors read at scrape time, deleting a series, Summary quantiles | exporter, scheduler | awkward | prometheus Collector, GaugeFunc, Summary | S to M |
| 11 | HTTP server: read and idle timeouts, header limits, a body limit per route, Request host, TLS state, raw query and escaped path, HTML error pages, Redirect | gateway, web app | awkward | net/http Server, MaxBytesReader, Request.Host, URL.RawQuery | S each |
| 12 | Text formatting: width, padding, zero padding, fixed precision floats; an aligned table writer | CLI | awkward | fmt verbs, strconv.FormatFloat, text/tabwriter | S |
| 13 | A priority queue with Remove and Fix | scheduler | awkward | container/heap | S |
| 14 | A logger that carries fields (Logger.With) | scheduler | awkward | log/slog | S |
| 15 | Rate limiting and backoff closed 2026-10-29 (std/rate, std/backoff); migrations open. Rate limiting (token bucket), backoff, migrations | gateway, web app, scheduler | awkward | x/time/rate, cenkalti/backoff, golang-migrate (all outside Go's std) | S each |
| 16 | Closed 2026-10-29 (walk_dir, FileInfo.is_symlink, create_dir_all_mode, write_atomic, toml.encode, cli float_flag). Fs walk with SkipDir and early stop, the symlink bit in FileInfo, a mode on create_dir_all and atomic writes; a TOML encoder; a float flag | CLI | awkward | filepath.WalkDir, os.WriteFile, BurntSushi/toml | S each |
| 17 | Time.truncate, a Weekday number, a ticker | scheduler | minor | time | S |

Language obstacles the services met (not std, for the language owners):
a sort with a comparator (`sort_by` takes a key only, so a descending
`Str` sort cannot be written); `T??` does not parse (XO0160), so "leave
this optional field unchanged" is `Option[T?]`; no storable cancel
handle for one task (context.WithCancel); `after (expr) =>` in a
`select` parses as a call to `after`; no literal for json.Value objects.

Found wrong in the spec: spec/std.md section 27's TLS example returns
`Err(Error.msg("server refused TLS"))`, but Error has no `msg` (XO0601);
the services had to declare their own message error type, as Go's
`errors.New` would give them.

Proposed for 0.35 (the blockers that are small or that unblock a class
of service, in order): 1, 2, 3, 4, 7, 5, 6, then a small batch of 12, 13
and 9 (Hostname, Getpid). Cron (8) belongs in a module outside std, as
it is outside Go's.

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
| 7 | TLS on a Conn, SMTP, mail addresses | 2 of 5 | crypto/tls, net/smtp, net/mail (all done 2026-10-20) |
| 8 | JSON key names (keywords, camelCase) | 1 of 5 (most third party payloads) | encoding/json struct tags |
| 9 | Multipart, MIME types, content sniffing, ServeContent, streaming request bodies | 1 of 5 | mime, mime/multipart, net/http |
| 10 | Public RSA and ECDSA verify, big integers | 1 of 5 | crypto/rsa, crypto/ecdsa, math/big |
| 11 | UUID | 1 of 5 | google/uuid |
| 12 | Int to text in other bases, Float math, embed, XML, client IP parsing | 1 of 5 each | strconv, math, embed, encoding/xml, net/netip |
