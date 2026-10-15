# 0074: std/http, std/json, and std/log on the native backends; std written in Xo

Status: Accepted (2026-10-08); amended by 0083 (http.stream, std/crypto, std/sql,
and test binaries on the native backends) and by 0105 and 0106 (TLS: the
"no TLS" difference below no longer holds). Implemented in `std/native/*.xo`
(implementation modules), `std/llrt/xo_std.c`, `xo_json.c`, `xo_http.c`,
`xo_unicode.c` (C intrinsics), `internal/check/load.go` (`loadNative`),
`internal/ssa/build_native.go`, `build_show.go`, `build_json.go`. Tests:
`internal/check/native_test.go`; the `native_*` and `http_*` goldens of
`internal/llvm/testdata/golden` (Go, LLVM, and arm64 must agree, no
allocation live at exit, also with share probes and under
ThreadSanitizer with 1, 2, and per CPU workers in TestGoldenTasks, and
under AddressSanitizer and UBSan in TestGoldenASan); `json_diff_test.go`
(TestJSONParseDiff); `http_diff_test.go` (TestHTTPParseDiff, FuzzHTTPDiff
for `go test -fuzz`, TestHTTPFuzz; since 2026-10-15 they drive Xo
programs built with the native backends, the C drivers being gone with
the C parsers); `unicode_test.go`.

## Context
The native backends (decisions 0046, 0049, 0068 to 0072) had tasks,
fibers, an event loop, TCP sockets, and Map, but no std/http, std/json,
or std/log, and few prelude methods (List had push, pop, set, get, len;
Str had len and is_empty). A server like bench/perf/http/xo also needs
`main ! Error`, `requires ... else`, Display of user types and Durations,
`==` on maps, Int.parse, and the closure methods of List. The Go backend
implements std in Go (std/rt) on top of net/http and encoding/json; the
native backends have no Go to call, and their runtime interface passes
only scalars and pointers (decision 0046).

Writing every std function as hand built SSA (as build_map.go does for
Map) is slow to write and hard to read; writing it all in C needs the
type of every value (decision 0046 rejects layouts in the runtime).

## Decision
1. **std implemented in Xo for the native backends.** `std/native/<name>.xo`
   implements `std/<name>` (`prelude.xo` the prelude). The checker loads
   them only for a native build (`check.Options.Native`, `build.CheckNative`):
   the prelude's and those of the std modules the program uses, checked like
   user modules (internal/check's TestNativeModules requires no diagnostic).
   The SSA builder lowers a std function or method that has an
   implementation to a call of it, before any lowering of its own: a method
   of a type the module declares too (`Router.handle`), else the function
   `<type>_<name>` with the receiver first (`str_trim`, `list_any`,
   `request_param`), else the function of the same name. The implementation
   has the declaration's type parameters in order. A std struct the module
   declares too is lowered as that struct: opaque handles get fields
   (`Router`, `Server`, `Client`, `Key`) and `Request` gets hidden ones
   (route parameters, typed values). Missing arguments take the
   implementation's defaults; a trailing parameter of type `rt.Loc` gets the
   `file:line` of the call (faults located at the caller, like std/rt's
   FaultHere).
2. **Intrinsics.** `std/native/rt.xo`, `rtjson.xo`, `rthttp.xo` declare
   functions the implementations call (`use native/rt`; user modules cannot
   import them). A call is `rt.<name>` (the C function `xo_<name>`); Float
   arguments and results, Str results (the object, its length through a
   pointer), and `rs_*` functions that append to their first Str (consumed)
   are new in both backends' runtime call lowering. Opaque structs there are
   runtime handles (`Srv`, `HConn`, `CResp`, `Any`). Generic intrinsics are
   lowered by the builder: `json_enc`/`json_dec` (codecs per type),
   `any_of`/`any_get` (a value with its type name, for request values),
   `status_of` (whether the error type has `status`, http.StatusError),
   `launder1`/`launder2` (a handler's effects are counted where it is
   registered, std.md 4.1), `dyn_text`/`dyn_raw`.
3. **`Result[T, Never]` is T** in the builder (a generic implementation
   instantiated with a function that cannot fail): `?` on it is the value,
   `Ok(x)` is x, `Err` and its patterns never happen.
4. **Display and Debug** (build_show.go): interpolation of any type
   (a `display` method, a std type's implementation, else the Debug text of
   `derive Display`); Debug text from functions generated per checker type
   in the Go backend's format; Duration (time.Duration.String) and Time
   (RFC 3339 nano, UTC). A value of the prelude interfaces Error, Display,
   or Debug is its text (`xo.dyn {text, raw}`, made at the conversion), so
   `main ! Error` prints `error: <text>` and exits 1 (no frames), and log
   fields of `Map[Str, Debug]` print as on the Go backend. Other interface
   types stay unsupported.
5. **JSON** (build_json.go, xo_json.c): codecs generated per type with the
   output of std/rt's JsonEnc and internal/gen's XoJsonEnc and XoJsonDec
   (field order, Patch, enums, Option, List, Set, Map[Str, V], Duration,
   Time, Bytes as base64, floats as encoding/json). Decoding parses into a
   tape in C that reproduces std/rt's jsonParse (encoding/json's
   Decoder.Token) and its error texts; the decoders walk it, and an error's
   path is built only when decoding fails (relative paths, prefixed by each
   caller). TestJSONParseDiff compares the parser with jsonParse on a corpus
   and 20000 random valid and mutated texts.
6. **HTTP/1.1 server** (xo_http.c with std/native/http.xo): reading and
   writing reproduce net/http as std/rt's serve uses it (the whole body read
   before the handler, the response written at once): request line and MIME
   header with continuation lines, validation and error answers (400 with
   and without a reason, 431 past 1 MiB + 4096 bytes of head or 10000
   fields, 501 for transfer encodings other than chunked, 505, 417),
   Content-Length and chunked bodies (trailers read and dropped), Expect:
   100-continue, keep-alive and HTTP/1.0 rules, pipelining, Host and
   absolute targets, ParseQuery; responses with the handler's header
   sorted, Date, Content-Length when the body fits net/http's 2048 byte
   buffer and chunked (2048 byte chunks) above, sniffed Content-Type,
   Connection. Routing, middleware (`wrap`, `mount`), request values, error
   mapping (section 4.2) are Xo code like std/rt's. Each connection is a task
   in a scope of the server's own under main's root, so canceling serve
   lets requests in flight finish (std/rt's reqRoot); serve then closes
   idle connections and waits at most 30 s (after a stop request: until
   the grace period ends). TestHTTPParseDiff sends the same bytes to the C
   code and to a Go net/http server and compares the answers.
7. **HTTP client** (xo_http.c `cl_do`): one connection per request, the
   request line and header as net/http's Transport writes them (Host,
   User-Agent: Go-http-client/1.1, Content-Length, sorted fields,
   Accept-Encoding: gzip), the response read as ReadResponse does, redirects
   as Client.Do follows them, the client's timeout over the whole exchange,
   errors as std/rt maps them (Connect, Timeout, Protocol, InvalidUrl).
8. **std/log**: text and JSON records to standard error as std/rt writes
   them; `configure` sets the level and format.

## Consequences
- bench/perf/http/xo builds with `--backend=arm64` and `--backend=llvm`
  and answers like the Go backend build. Smoke check (2026-10-08, no
  timing): each native build answered about 360 sequential requests
  (create, get, patch, list, duplicate, bad JSON, delete, get after
  delete) with 303 of 303 checks right, then 8 concurrent clients made
  200 users with 200 distinct ids, and SIGTERM ended the server with exit
  status 0. `bench/perf/http/run.sh` has configs `xo-arm64` and `xo-llvm`
  for the throughput comparison, to be run on a quiet machine.
- Fuzzing (2026-10-08): http_fuzz.c ran about 60 million mutated inputs
  through the server's and client's parsers and the JSON parser under
  AddressSanitizer and UBSan (three seeds, 60 s each) after the fix of one
  overflow it found (a 16 digit chunk size); TestHTTPParseDiff agreed with
  net/http on 30000 random connections for each of 16 seeds after the
  quirks in `gotchas/net-http-quirks-to-copy.md` were copied; FuzzHTTPDiff
  found two more differences (net.SplitHostPort's checks of brackets,
  requestTooLarge when 256 KiB of a body never came), fixed and kept as
  seeds in `internal/llvm/testdata/fuzz`.
- Not tuned yet: a request allocates per header field, per path segment
  and route while dispatching, and per response field; the throughput
  comparison will show where it matters.
- Differences from the Go backend, all documented here: a fault in a
  handler exits the process (decision 0070: no unwinding of faults), not a
  500; a request body longer than 64 MiB (XO_HTTP_MAX_BODY) is cut short
  and the connection closed (Go has no limit); more than 10000 header
  fields is 431; no TLS, so no HTTP/2 and no https client (an error);
  `http.stream` is not implemented (a build error); the client does not
  pool connections and does not decompress gzip (Xo servers never send it);
  `strings.ToLower` and ToUpper use Go's Unicode case table (generated,
  TestUnicodeTables), but strconv.Quote's IsPrint is approximated; a 500
  for an error without `status` is logged without frames; header maps of
  requests keep the order of first appearance (the Go backend's order is a
  Go map's); the error of an interface typed value (`Error`) carries only
  its text, so `status_of` cannot see through it.
- `==` on values holding a Map or Set is lowered (same entries in any
  order, xo.eqc); a Request's hidden fields take part in `==` natively.
- Implementations are ordinary Xo: new std surface for the native backends
  is mostly Xo code plus small C intrinsics.

## Alternatives
- **Hand built SSA for all of std** (the build_map.go way): no checker
  changes, but thousands of lines of SSA construction for routing and
  middleware, with no type checking of the logic.
- **The whole server in C with callbacks into Xo**: fast, but C would need
  each handler's closure layout and the Request's layout.
- **Implementations in the std declaration files** (bodies next to the
  signatures): the Go backend reads those files too and would have to
  ignore the bodies; separate modules keep the Go build unchanged.
- **A generic `Any` through closures** for request values: closures cannot
  give their captured value back at a known type; a list of one with its
  type name and a typed drop function can.
