# 0259: External bytes becoming Str

Status: Proposed (Xo Lang 3, 2026-10-18; for the owner: a language
invariant)

## Context

spec/core.md 3: `Str` is "Immutable UTF-8 text", and slicing a `Str`
inside a UTF-8 sequence faults (3.6). Nothing enforces the first rule at
the edges of a program: the native std builds `Str` from bytes it read
with unvalidated conversions (`raw.str_at`, `raw.str_slice`,
`rt.cast[Bytes, Str]`), so a program can hold a `Str` that is not UTF-8,
and an ordinary string operation then faults on it.

FuzzHTTPDiff found it on 2026-10-18 (the stabilization fuzzing; the 0.19.0
seed behaves the same, so it is old): the request
`0 /status/\x82 HTTP/1.0` reaches the handler with `req.path` holding the
byte 0x82, the diff program's `req.path[8..]` faults (byte 8 is a
continuation byte), and the server answers 500 where net/http answers 200
(Go's strings are bytes; nothing faults). The input is kept as a skipped
known failure (internal/llvm http_diff_test.go FuzzHTTPDiff skips a
request head that is not UTF-8, naming this decision;
testdata/fuzz/FuzzHTTPDiff/8d7429a4e28ec7ca).

TestHTTPFuzz/client found the same class inside std: a redirect whose
Location host starts with continuation bytes (`http://\x82\x82:1/x`)
reached netpolicy.split_hostport through the dial, and its Str slice
faulted, ending the client (exit 70). That one is fixed now, whatever
this decision says (std must not fault on outside input): netpolicy
copies the bytes (raw.str_slice) instead of slicing the Str;
TestHTTPClientRedirectNotUTF8 is its regression test.

Where external bytes become `Str` today (native std, 2026-10-18):

| Source | API | Today |
|---|---|---|
| HTTP request line | `Request.method`, `path`, `url.path`, `url.host` | raw bytes (http1.xo parse_head, str_slice) |
| HTTP query | `Request.query`, `url.query` | percent-decoded to raw bytes (neturl.xo): `%82` makes an invalid Str even from an ASCII request |
| HTTP header values (and names) | `Request.header`, `headers`; the client's `Response.header` | raw bytes; RFC 9110 allows obs-text (0x80 to 0xFF) in values |
| HTTP/2 | the same fields from HPACK (hpack.xo, h2srv.xo) | raw bytes, Huffman decoded or literal |
| Path parameters | `Request.param` | slices of the raw path |
| Request and response bodies as text | `Response.text()` | validated: None when not UTF-8 |
| Files | `Fs.read_text`, `File.read_line` | raw bytes (prelude.xo fs_read_text: a cast; fsops.xo) |
| File names | `Fs.read_dir`, `glob`, `read_link`, `cwd_path` | raw bytes (Unix names are bytes) |
| Standard input | `Stdio.read_line`, `read_all`, `lines`, `read_secret` | raw bytes (stdin.xo) |
| Child processes | `Child.read_line`; `run` output is Bytes | raw bytes for lines; Bytes elsewhere (callers use `to_str()`) |
| Environment and arguments | `Env.get`, `vars`, `args`, `program`, `home` | raw bytes (process.xo cstr) |
| CLI flags | `cli` parsed values | from args: raw |
| JSON | `json.decode` strings | validated: invalid UTF-8 and lone surrogates become U+FFFD (jsontext.xo, as Go's encoding/json) |
| Archives | tar names, pax records, zip names | raw bytes (archive.xo parse_cstr, parse_pax, zip headers); one path validates |
| TLS server name | used inside tlssrv for certificate choice | raw bytes, not exposed |
| DNS | names are inputs; answers are addresses | not a source |
| Sockets | `Conn.read` | Bytes (not a source) |
| `Bytes.to_str()` | the program's own conversion | validated: None when not UTF-8 |

So the language has two answers already (None, and U+FFFD for JSON) and
a dozen boundaries with none.

## Options

**(a) Reject.** A boundary that would produce a non-UTF-8 `Str` fails
instead: HTTP answers 400 Bad Request for a request line or header with
such bytes (the fuzz input: 400); `read_text` and `read_line` return an
error (a new `FsErr`/`IoErr` case); `Env.get` returns None for such a
value and `vars` leaves it out.
- Routing: never sees such a request. Logging: the 400 is logged as a
  bad request. Signatures: new error cases (Fs and Io error enums
  gain `NotUtf8`), no new functions.
- Versus net/http: Go serves these requests (and header values with
  obs-text, which RFC 9110 permits, are refused here). An
  internationalized legacy client sending Latin-1 headers breaks.

**(b) Replace.** Every boundary replaces each invalid sequence with
U+FFFD, as JSON already does (Go's `strings.ToValidUTF8` with "�",
WHATWG's decoder): `req.path` becomes `/status/\u{FFFD}`.
- Routing: a route still matches on the valid parts; two different
  invalid paths can map to the same `Str` (lossy). Logging: shows the
  replacement character. Signatures: none change; the exact bytes stay
  reachable where an API also offers `Bytes` (`Fs.read`, stdin's
  `read_all_bytes`); env, args, header values, and paths would need
  `_bytes` twins to recover exact bytes.
- Versus net/http: the handler sees U+FFFD where Go sees the raw byte;
  echoing a header value back changes it.

**(c) Escape.** URL parts keep their bytes by percent-encoding the
invalid ones (`/status/%82`; a query value's `%82` stays `%82` instead of
being decoded to a raw byte); other sources use (a) or (b).
- Routing: the path is still a valid URL path, routes match as Go's
  would on `RawPath`; a pattern segment receives `%82`, which the
  program can decode to Bytes itself. Logging: shows the escape.
  Signatures: none.
- Versus net/http: Go's `URL.Path` holds the raw byte and `RawPath` the
  escaped form; Xo's `url.path` would match Go's `EscapedPath()` for
  such paths only (valid UTF-8 paths are unchanged, as today).

**(d) Relax the invariant** (Go's model): `Str` is bytes that are UTF-8
by convention; slicing never faults; `runes()` and printing handle
invalid bytes as Go does (U+FFFD per bad byte when decoding).
- Routing and logging unchanged; no boundary work at all; matches
  net/http exactly.
- Versus the language: a core guarantee goes (spec 3, the slicing
  fault, every API that promises text), and the checker's string
  invariants (length in runes, `Str` keys) become runtime questions.

## Decision (proposed, for the owner)

(c) for URL paths and query values (percent-encode invalid bytes, never
decode a percent escape to a non-UTF-8 byte), (b) for every other
boundary in the table (HTTP header values and names, files read as text,
file names, stdin, child lines, env and args, archive names), keeping
or adding a `Bytes` API wherever exact bytes matter (`Fs.read` and
stdin's bytes already exist; add `Env.get_bytes` and
`Request.header_bytes` only when someone needs them). One helper in
std/native (`text.valid(b: Bytes) -> Str`, U+FFFD replacement, with a
fast path for ASCII and valid input) is the only way native std turns
external bytes into `Str`; a repotest check lists the unvalidated
conversions (`raw.str_at` and casts on external data) so a new boundary
cannot skip it.

Why: (c) keeps HTTP routing and URLs exact without refusing requests Go
serves; (b) is what JSON already does, never refuses, and needs no new
error cases; (a) refuses legitimate RFC 9110 traffic; (d) gives up the
invariant the rest of the language relies on.

## Consequences

- The fuzz input's known failure turns into a passing case once the
  diff test's Go handler applies the same normalization (escape the
  path, U+FFFD in header values); FuzzHTTPDiff and TestHTTPFuzz then
  cover the rule.
- Performance: one validation pass over each external text (a word at a
  time for ASCII), small next to parsing; HTTP's common names (http1
  common_name) and ASCII-only requests take the fast path.
- The Go backend (std/rt) follows the same rule at the same boundaries,
  so native and Go programs agree (Go's own `string` stays bytes
  underneath).
- spec/std.md documents, per API, that invalid input is escaped or
  replaced; spec/core.md 3's "UTF-8 text" becomes a guarantee.

## Alternatives

Listed above as (a), (a) only for HTTP with (b) elsewhere, and (d). A
per-API choice without one helper was considered and rejected: the
table shows that is how the gaps arose.
