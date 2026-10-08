# Xo Standard Library and Prelude

Status: draft 0.6 (2026-10-08)
Companion to `spec/core.md`.

Signatures are written in Xo syntax. `[var]` before a receiver means the method
mutates and needs a `var` binding (core 3.4). Effects are listed with `uses`.
Everything here is part of the 2026 edition compatibility promise once the
edition is released.

## 0. Conventions

- **Capabilities and fakes are handles.** Copying a `Net`, `Fs`, `Clock`, ...
  or a test fake shares the same underlying resource. They can be captured in
  closures and stored in structs.
- **Every error type implements `Display`.** So any std error converts to
  `Error` at a `?` site.
- **Every error type returned by a blocking operation has a `Canceled`
  variant** (core 7.2).
- Functions that cannot fail have no `!`. Lookups that may miss return `T?`.
- Braces are literal in plain strings (interpolation is `${x}`), so route
  patterns are ordinary strings: `"GET /users/{id}"`.
- Fields of std structs are `pub`: they can be read and the structs can be
  constructed (useful for testing pure functions).

---

## 1. Prelude interfaces

Available everywhere without `use`.

```
interface Display { fn display(self) -> Str }
interface Error   { fn display(self) -> Str }   // any Display type is an Error
interface Debug   { fn debug(self) -> Str }     // implemented for every type
```

`Eq`, `Hash`, and `Ord` are builtin interfaces with no user written methods.
`Eq` and `Hash` are automatic when all fields implement them; `Ord` comes from
`derive Ord` (field order, then variant order). Builtin scalar types, `Str`,
`Duration`, and `Time` are `Ord`. `Float` is `Ord` with a total order in which
NaN sorts after every other value.

`Int`, the sized integers, `Float`, `Bool`, `Rune`, `Str`, `Duration`, `Time`,
and every std error type implement `Display`. `Bytes` does not (use
`to_str()`). `Json` is implemented by `derive Json` and by
builtin types (section 5).

Every `Ord` type has `a.min(b)`, `a.max(b)`, `a.clamp(lo, hi)`.

When a value is passed where an interface type is expected (for example a
`Map[Str, Debug]` literal), it is converted to that interface implicitly
(core 2.10).

---

## 2. Prelude methods on builtin types

### 2.1 Numbers

```
Int.parse(s: Str) -> Int?
Float.parse(s: Str) -> Float?
Int(x: Float) -> Int                 // truncates toward zero; faults on NaN or out of range
Float(x: Int) -> Float
Int32(x: Int) -> Int32               // faults out of range; same for every sized type
Int32.try(x: Int) -> Int32?

fn (n: Int) abs() -> Int
fn (n: Int) pow(e: Int) -> Int
fn (n: Int) wrapping_add(m: Int) -> Int      // also wrapping_sub, wrapping_mul
fn (n: Int) saturating_add(m: Int) -> Int    // also saturating_sub, saturating_mul
fn (n: Int) checked_add(m: Int) -> Int?      // also checked_sub, checked_mul, checked_div

fn (f: Float) floor() -> Float               // also ceil, round, trunc, abs, sqrt
fn (f: Float) is_nan() -> Bool
```

### 2.2 Str, Rune, Bytes

`Str` is UTF-8. `+` concatenates `Str` and `Bytes`.

```
fn (s: Str) len() -> Int                     // bytes
fn (s: Str) rune_count() -> Int
fn (s: Str) is_empty() -> Bool
fn (s: Str) contains(sub: Str) -> Bool
fn (s: Str) starts_with(p: Str) -> Bool
fn (s: Str) ends_with(p: Str) -> Bool
fn (s: Str) find(sub: Str) -> Int?           // byte offset
fn (s: Str) split(sep: Str) -> List[Str]
fn (s: Str) lines() -> List[Str]             // splits on \n, strips \r
fn (s: Str) trim() -> Str                    // also trim_start, trim_end
fn (s: Str) trim_prefix(p: Str) -> Str       // also trim_suffix
fn (s: Str) to_lower() -> Str                // also to_upper (Unicode aware)
fn (s: Str) replace(old: Str, new: Str) -> Str
fn (s: Str) repeat(n: Int) -> Str
fn (s: Str) runes() -> List[Rune]
fn (s: Str) bytes() -> Bytes

fn (b: Bytes) len() -> Int
fn (b: Bytes) is_empty() -> Bool
fn (b: Bytes) to_str() -> Str?               // None if not valid UTF-8
fn (b: Bytes) to_hex() -> Str                // lowercase
fn (b: Bytes) to_base64() -> Str             // standard encoding, padded
Bytes.from_hex(s: Str) -> Bytes?
Bytes.from_base64(s: Str) -> Bytes?

fn (r: Rune) is_digit() -> Bool              // also is_letter, is_space, to_lower, to_upper
```

Slicing (`s[a..b]`) is in core 3.6.

### 2.3 List[T]

```
fn (xs: List[T]) len() -> Int
fn (xs: List[T]) is_empty() -> Bool
fn (xs: List[T]) get(i: Int) -> T?
fn (xs: List[T]) first() -> T?               // also last
fn (xs: List[T]) contains(x: T) -> Bool      // T: Eq
fn (xs: List[T]) index_of(x: T) -> Int?      // T: Eq
fn (xs: List[T]) indexed() -> List[(Int, T)]
fn (xs: List[T]) map[U](f: fn(T) -> U) -> List[U]
fn (xs: List[T]) filter(f: fn(T) -> Bool) -> List[T]
fn (xs: List[T]) fold[A](init: A, f: fn(A, T) -> A) -> A
fn (xs: List[T]) find(f: fn(T) -> Bool) -> T?
fn (xs: List[T]) any(f: fn(T) -> Bool) -> Bool   // also all
fn (xs: List[T]) sort() -> List[T]           // T: Ord, stable
fn (xs: List[T]) sort_by[K: Ord](key: fn(T) -> K) -> List[T]
fn (xs: List[T]) reverse() -> List[T]
fn (xs: List[T]) to_set() -> Set[T]
(0..n).to_list()                             // Range[Int] to List[Int]
fn (xs: List[Str]) join(sep: Str) -> Str
xs + ys                                      // concatenation

fn ([var] xs: List[T]) push(x: T)
fn ([var] xs: List[T]) pop() -> T?           // removes the last element
fn ([var] xs: List[T]) insert(i: Int, x: T)
fn ([var] xs: List[T]) remove_at(i: Int) -> T
fn ([var] xs: List[T]) set(i: Int, x: T)     // also xs[i] = x on a var list
```

Higher order methods (`map`, `filter`, ...) pass through the callback's effects
and errors through effect variables (core 6.2); `map` with a failing callback
returns `List[U] ! E`.

### 2.4 Map[K, V] and Set[T]

Insertion ordered. `put` on an existing key keeps its position.

```
fn (m: Map[K, V]) len() -> Int
fn (m: Map[K, V]) is_empty() -> Bool
fn (m: Map[K, V]) get(k: K) -> V?
fn (m: Map[K, V]) get_or_fault(k: K, reason: Str) -> V   // faults with the key and reason when missing
fn (m: Map[K, V]) contains(k: K) -> Bool
fn (m: Map[K, V]) keys() -> List[K]
fn (m: Map[K, V]) values() -> List[V]
fn (m: Map[K, V]) entries() -> List[(K, V)]
fn (m: Map[K, V]) filter(f: fn(K, V) -> Bool) -> Map[K, V]

fn ([var] m: Map[K, V]) put(k: K, v: V)
fn ([var] m: Map[K, V]) remove(k: K) -> V?
fn ([var] m: Map[K, V]) update[R](k: K, init: V, f: fn(var V) -> R) -> R
    // reads k (or init if missing), runs f, stores the result, returns f's value

fn (s: Set[T]) len() -> Int
fn (s: Set[T]) contains(x: T) -> Bool
fn (s: Set[T]) union(o: Set[T]) -> Set[T]    // also intersect, difference
fn (s: Set[T]) to_list() -> List[T]
fn (s: Set[T]) is_empty() -> Bool
fn (s: Set[T]) any(f: fn(T) -> Bool) -> Bool   // also all
fn (s: Set[T]) filter(f: fn(T) -> Bool) -> Set[T]
fn (s: Set[T]) map[U](f: fn(T) -> U) -> Set[U]
fn ([var] s: Set[T]) add(x: T)
fn ([var] s: Set[T]) remove(x: T) -> Bool
```

Iterating a `var` collection while mutating it iterates the value as it was
when the loop started.

### 2.5 Option[T] and Result[T, E]

```
fn (o: T?) is_some() -> Bool                 // also is_none
fn (o: T?) or_err[E](e: E) -> Result[T, E]
fn (o: T?) map[U](f: fn(T) -> U) -> U?
fn (o: T?) and_then[U](f: fn(T) -> U?) -> U?
fn (o: T?) or_fault(msg: Str) -> T           // faults on None
o ?? default                                 // T

fn (r: Result[T, E]) is_ok() -> Bool         // also is_err
fn (r: Result[T, E]) map[U](f: fn(T) -> U) -> Result[U, E]
fn (r: Result[T, E]) map_err[F](f: fn(E) -> F) -> Result[T, F]
fn (r: Result[T, E]) ok() -> T?
fn (r: Result[T, E]) err() -> E?
fn (r: Result[T, E]) or_fault(msg: Str) -> T // faults on Err, message includes the error
fn (r: Result[T, E]) ignore_err(reason: Str) -> T?
    // the only way to discard an error (core 2.6); logs the error and reason at debug level
```

### 2.6 Duration and Time

Operators are in core 2.1. `Duration` displays as `1.5s`, `250ms`; `Time`
displays as RFC 3339 in UTC.

```
Duration.from_secs(s: Float) -> Duration
Duration.from_millis(ms: Int) -> Duration
Duration.from_nanos(ns: Int) -> Duration
fn (d: Duration) seconds() -> Float
fn (d: Duration) millis() -> Int             // truncated
fn (d: Duration) nanos() -> Int
fn (d: Duration) scale(f: Float) -> Duration // rounds to the nearest nanosecond
fn (d: Duration) ceil_seconds() -> Int
fn (d: Duration) abs() -> Duration           // durations may be negative

Time.UNIX_EPOCH: Time
Time.from_unix(secs: Int) -> Time
Time.from_unix_millis(ms: Int) -> Time
Time.parse_rfc3339(s: Str) -> Time?
fn (t: Time) unix() -> Int
fn (t: Time) unix_millis() -> Int
fn (t: Time) format_rfc3339() -> Str
```

### 2.7 Concurrency and state types

```
Chan[T].new(cap: Int = 0) -> Chan[T]
fn (c: Chan[T]) send(x: T)                   // blocks; cancellation point
fn (c: Chan[T]) recv() -> T?                 // None when closed and drained
fn (c: Chan[T]) try_send(x: T) -> Bool         // false if full or closed; never blocks
fn (c: Chan[T]) try_recv() -> T?             // None if empty; never blocks
fn (c: Chan[T]) close()
fn (c: Chan[T]) len() -> Int

Mutex[T].new(v: T) -> Mutex[T]
fn (m: Mutex[T]) with[R, E, fx: Effects](f: fn(var T) -> R ! E uses fx) -> R ! E uses fx

Atomic[T].new(v: T) -> Atomic[T]             // T is Int or Bool
fn (a: Atomic[T]) get() -> T
fn (a: Atomic[T]) set(v: T)
fn (a: Atomic[Int]) add(n: Int) -> Int       // returns the new value
fn (a: Atomic[Int]) max(n: Int) -> Int       // stores the larger value, returns it
fn (a: Atomic[T]) swap(v: T) -> T
fn (a: Atomic[T]) compare_and_swap(old: T, new: T) -> Bool

fn (t: Task[T, E]) await() -> T ! E          // written `t.await`

Buf[T].new() -> Buf[T]                       // also with_capacity(n)
fn ([var] b: Buf[T]) push(x: T)
fn (b: Buf[T]) freeze() -> List[T]           // moves b
StrBuilder.new() -> StrBuilder
fn ([var] b: StrBuilder) write(s: Str)
fn (b: StrBuilder) freeze() -> Str           // moves b

Secret[T].new(v: T) -> Secret[T]
fn (s: Secret[T]) reveal() -> T
```

---

## 3. Capabilities

Predeclared interfaces. `main(os: Os)` receives the real ones; tests receive
fakes (section 8).

### 3.1 Fs (`uses fs.read` / `fs.write`)

Paths use `/` on every OS. `os.fs` is rooted at the filesystem root, accepts
absolute paths (`/home/me`, `C:/data`), and resolves relative paths against
the working directory. In a sub capability, paths are relative to its root,
`"."` and `"/"` both mean the root, and a path that escapes it (`..`) fails
with `Denied`.

```
enum FsErr {
  NotFound(path: Str)
  Exists(path: Str)
  NotEmpty(path: Str)
  NotDir(path: Str)
  IsDir(path: Str)
  Denied(path: Str)
  Io(path: Str, msg: Str)
  Canceled
}

struct FileInfo {
  name: Str              // last path segment
  path: Str              // relative to the capability root
  is_dir: Bool
  size: Int
  modified: Time
}

fn (f: Fs) sub(dir: Str) -> Fs                               // lazy: dir need not exist yet
fn (f: Fs) cwd() -> Fs                                       // sub capability at the working directory
fn (f: Fs) read_only() -> Fs
fn (f: Fs) write_only() -> Fs                // reads fail with Denied
fn (f: Fs) case_sensitive() -> Bool                          // of the underlying filesystem
fn (f: Fs) stat(path: Str) -> FileInfo? ! FsErr  uses fs.read // None when missing
fn (f: Fs) exists(path: Str) -> Bool ! FsErr     uses fs.read
fn (f: Fs) read(path: Str) -> Bytes ! FsErr      uses fs.read
fn (f: Fs) read_text(path: Str) -> Str ! FsErr   uses fs.read
fn (f: Fs) read_dir(dir: Str) -> List[FileInfo] ! FsErr uses fs.read   // sorted by name
fn (f: Fs) walk(dir: Str) -> List[FileInfo] ! FsErr     uses fs.read
    // recursive, parents before children, siblings by name; excludes `dir` itself;
    // fails with NotFound if `dir` does not exist;
    // paths are relative to the capability root with no "./" prefix
fn (f: Fs) write(path: Str, data: Bytes) -> () ! FsErr  uses fs.write  // create or truncate
fn (f: Fs) create_temp(dir: Str, prefix: Str) -> Str ! FsErr uses fs.write
    // returns the new file's path, relative to the capability root
fn (f: Fs) rename(from: Str, to: Str) -> () ! FsErr     uses fs.write  // atomic, replaces a file
fn (f: Fs) remove(path: Str) -> () ! FsErr              uses fs.write  // file or empty dir
fn (f: Fs) remove_all(path: Str) -> () ! FsErr          uses fs.write
fn (f: Fs) create_dir_all(dir: Str) -> () ! FsErr       uses fs.write
fn (f: Fs) set_modified(path: Str, t: Time) -> () ! FsErr uses fs.write
```

Streaming file handles are planned for a later draft.

### 3.2 Net (`uses net.dial` / `net.listen`)

```
enum NetErr { InvalidAddr(msg: Str), Refused, Timeout, Io(msg: Str), Canceled }

fn (n: Net) listen(addr: Str) -> Listener ! NetErr uses net.listen
fn (n: Net) dial(addr: Str) -> Conn ! NetErr       uses net.dial
```

Most code uses `std/http` instead of raw connections.

### 3.3 Clock (`uses clock`)

```
fn (c: Clock) now() -> Time
fn (c: Clock) sleep(d: Duration)             // cancellation point; unwinds when cancelled
fn (c: Clock) since(t: Time) -> Duration
```

### 3.4 Rand (`uses rand`)

Cryptographically secure for real capabilities, seeded and deterministic in
tests.

```
fn (r: Rand) int(lo: Int, hi: Int) -> Int    // in [lo, hi)
fn (r: Rand) float() -> Float                // in [0, 1)
fn (r: Rand) bool() -> Bool
fn (r: Rand) bytes(n: Int) -> Bytes
fn (r: Rand) shuffle[T](xs: List[T]) -> List[T]
```

### 3.5 Env (`uses env`)

```
enum EnvErr { Missing(name: Str) }

fn (e: Env) get(name: Str) -> Str?
fn (e: Env) vars() -> Map[Str, Str]
fn (e: Env) args() -> List[Str]              // without the program name
fn (e: Env) program() -> Str
fn (e: Env) home() -> Str ! EnvErr
```

### 3.6 Stdio (`uses stdio`)

```
enum IoErr { Closed, Io(msg: Str), Canceled }

fn (s: Stdio) print(text: Str)               // write errors are ignored
fn (s: Stdio) println(text: Str)
fn (s: Stdio) eprint(text: Str)
fn (s: Stdio) eprintln(text: Str)
fn (s: Stdio) write(data: Bytes) -> () ! IoErr
fn (s: Stdio) read_line() -> Str? ! IoErr    // None at end of input
fn (s: Stdio) read_all() -> Str ! IoErr
fn (s: Stdio) lines() -> List[Str] ! IoErr   // reads all input
```

### 3.7 Proc (`uses proc`)

```
struct ProcOutput { code: Int, stdout: Bytes, stderr: Bytes }
enum ProcErr { NotFound(cmd: Str), Io(msg: Str), Canceled }

fn (p: Proc) exit(code: Int) -> Never        // exits immediately; defer blocks do not run
fn (p: Proc) set_grace(d: Duration)          // time between stop requested and cancel (default 30s)
fn (p: Proc) run(cmd: Str, args: List[Str]) -> ProcOutput ! ProcErr
```

Returning `Err` from `main` prints the error with its frames to standard error
and exits with status 1. Prefer that over `exit` for failures.

---

## 4. std/http

### 4.1 Routing

```
pub fn router() -> Router
fn ([var] r: Router) handle[E: Error, fx: Effects](pattern: Str, h: fn(Request) -> Response ! E uses fx) uses fx
fn ([var] r: Router) wrap[fx: Effects](m: fn(Request, Next) -> Response uses fx) uses fx
fn (r: Router) call(req: Request) -> Response      // in process dispatch, for tests
type Next = fn(Request) -> Response
```

- Registering a handler or middleware counts as performing its effects, so the
  function that builds the router declares them. This is where capabilities
  are captured.
- Pattern: `"[METHOD ]/path"`. Segments: literal, `{name}` (any segment),
  `{name:Int}` (must parse as `Int`, else 400 before the handler runs),
  `{name...}` (rest of the path). Without a method, any method matches.
  plain strings: `"GET /users/{id:Int}"`.
- Unmatched path: 404. Path matched with another method: 405.
- Middleware run in registration order, outermost first, and see the final
  response after error mapping.
- `Router`, `Next`, `call`, and `serve` carry no effects in their types:
  effects are accounted where handlers and middleware are registered, which
  is where their capabilities are captured.

### 4.2 Error to status mapping

```
interface StatusError {
  fn status(self) -> Int
  fn display(self) -> Str
}
```

When a handler returns `Err(e)`:

- if `e` implements `StatusError`, the response is `e.status()` with JSON body
  `{"error": e.display()}`;
- otherwise the response is 500 with `{"error": "internal error"}` and the
  error is logged with its frames.

A fault in a handler is isolated: 500, logged, crash bundle written.

### 4.3 Requests and responses

```
struct Request {
  method: Str
  path: Str
  url: Url
  headers: Map[Str, Str]     // canonical header keys, e.g. "Retry-After"
  body: Bytes
  remote_ip: Str
}
fn (r: Request) param(name: Str) -> Str             // faults if the pattern has no such segment
fn (r: Request) param_int(name: Str) -> Int         // segment must be declared {name:Int}
fn (r: Request) query(name: Str) -> Str?
fn (r: Request) header(name: Str) -> Str?           // case insensitive
fn (r: Request) json[T: Json]() -> T ! json.DecodeErr

pub fn new_request(method: Str, url: Str, headers: Map[Str, Str] = {}, body: Bytes = b"") -> Request
    // remote_ip is "127.0.0.1"; for tests and clients

struct Response {
  status: Int
  headers: Map[Str, Str]
  body: Bytes
}
pub fn json[T: Json](status: Int, v: T) -> Response   // sets Content-Type
pub fn text(status: Int, s: Str) -> Response
pub fn empty(status: Int) -> Response
pub fn stream[fx: Effects](status: Int, f: fn(BodyWriter) -> () ! IoErr uses fx) -> Response uses fx
    // the body is produced by `f` while the response is sent (for large exports)
fn (w: BodyWriter) write(data: Bytes) -> () ! IoErr
fn (w: BodyWriter) flush() -> () ! IoErr
fn (r: Response) with_header(name: Str, value: Str) -> Response
fn (r: Response) header(name: Str) -> Str?          // case insensitive
fn (r: Response) decode_json[T: Json]() -> T ! json.DecodeErr
```

### 4.4 Server

```
pub fn server(l: Listener, r: Router) -> Server
fn (s: Server) serve() -> () ! HttpErr uses net.listen
enum HttpErr { Io(msg: Str), Canceled }
```

`serve` runs until its scope is cancelled, then stops accepting, waits for
in flight requests, and returns `Ok(())`. Each request runs as an isolated
task with its own span and request id. To run a server alongside background
work:

```
scope s {
  s.spawn_background(fn() { run_evictor(limiter, os.clock) })
  srv.serve()?
}
```

### 4.5 Client

```
pub fn client(net: Net, timeout: Duration = 30s) -> Client
fn (c: Client) get(url: Str) -> Response ! ClientErr uses net.dial
fn (c: Client) send(req: Request) -> Response ! ClientErr uses net.dial
fn (c: Client) post_json[T: Json](url: Str, v: T) -> Response ! ClientErr uses net.dial

enum ClientErr {
  InvalidUrl(msg: Str)
  Connect(msg: Str)
  Timeout
  Protocol(msg: Str)
  Canceled
}

pub fn parse_url(s: Str) -> Url ! UrlErr
struct Url { scheme: Str, host: Str, port: Int?, path: Str, query: Map[Str, Str] }
struct UrlErr { msg: Str }
```

Any HTTP status is a successful `Response`; only transport failures are
`ClientErr`. The client does not retry. It follows up to 10 redirects for
`GET` and `HEAD`. `parse_url` accepts absolute RFC 3986 URLs with an
`http` or `https` scheme and a host; anything else is a `UrlErr`.

---

## 5. std/json

```
pub fn encode[T: Json](v: T) -> Bytes
pub fn encode_str[T: Json](v: T) -> Str
pub fn decode[T: Json](data: Bytes) -> T ! DecodeErr
pub fn decode_str[T: Json](s: Str) -> T ! DecodeErr

struct DecodeErr { path: Str, msg: Str }    // displays as "at $.user.email: expected string"

enum Patch[T] { Absent, Null, Value(v: T) } derive Json
fn (p: Patch[T]) apply_to(current: T) -> T    // Value(v) gives v; Absent and Null keep current
fn (p: Patch[T]) apply_opt(current: T?) -> T? // Absent keeps, Null clears, Value(v) sets
```

`derive Json` rules:

| Xo | JSON |
|---|---|
| struct | object, keys are field names |
| `T?` field | missing key or `null` decodes to `None`; `None` encodes as `null` |
| field with a default | missing key decodes to the default |
| other missing field | `DecodeErr` |
| `Patch[T]` field | missing key is `Absent`, `null` is `Null`, value is `Value(v)` |
| unknown keys | ignored |
| `Int`, `Float`, `Bool`, `Str` | number, number, boolean, string |
| `List[T]`, `Set[T]` | array |
| `Map[Str, V]` | object |
| enum unit variant | string `"Variant"` |
| enum variant with fields | object `{"Variant": {fields}}` |
| `Duration` | string `"1.5s"` |
| `Time` | RFC 3339 string |
| `Bytes` | base64 string |

Builtin types listed above implement `Json`, so `http.json(200, {"msg": "hi"})`
works without a struct.

---

## 6. std/log

Ambient: no capability, not an effect (core 6.1).

```
pub fn debug(msg: Str, fields: Map[Str, Debug] = {})   // also info, warn, error
pub fn configure(out: Stdio, level: Level = Level.Info, format: Format = Format.Text) uses stdio
enum Level { Debug, Info, Warn, Error }
enum Format { Text, Json }
```

```
log.info("listening", {"addr": addr, "port": port})
```

Every record includes the time, the current span, and the request id.
Without `configure`, logs go to standard error as text at `Info`.

---

## 7. std/task

```
enum Timed[E] { TimedOut, Failed(from E) } derive Display
enum Isolated[E] { Faulted(info: Fault), Failed(from E) } derive Display
struct Fault { msg: Str, location: Str, frames: List[Str] } derive Display

pub fn with_timeout[T, E, fx: Effects](clock: Clock, d: Duration, f: fn() -> T ! E uses fx) -> T ! Timed[E]
    uses (clock, fx)
    // runs f in a child scope; on expiry cancels it, waits for it to unwind, returns Err(TimedOut)

pub fn isolate[T, E, fx: Effects](f: fn() -> T ! E uses fx) -> T ! Isolated[E] uses fx

pub fn is_canceled() -> Bool
pub fn stop_requested() -> Bool          // the first shutdown signal arrived (core 7.5)
pub fn stopping() -> Chan[()]            // closed when a stop is requested

pub fn map_concurrent[T, R, E, fx: Effects](items: List[T], limit: Int, f: fn(T) -> R ! E uses fx)
    -> List[Result[R, E]] uses fx
    // runs f on every item with at most `limit` at once; results are in input order;
    // one failure does not stop the others (use with_timeout for a deadline)

struct Group {}                              // handle; see core 7.1
fn (g: Group) spawn[T, E, fx: Effects](f: fn() -> T ! E uses fx) -> Task[T, E] uses fx
fn (g: Group) spawn_background[E, fx: Effects](f: fn() -> () ! E uses fx) uses fx

pub fn map_concurrent_until[T, R, E, fx: Effects](clock: Clock, d: Duration, items: List[T], limit: Int,
    f: fn(T) -> R ! E uses fx) -> List[Result[R, Timed[E]]] uses (clock, fx)
    // like map_concurrent, but items not finished after d are cancelled and give
    // Err(TimedOut); finished results are kept, in input order

pub fn once[T, E]() -> Once[T, E]           // Once is a handle: copies share it
fn (o: Once[T, E]) get[fx: Effects](f: fn() -> T ! E uses fx) -> T ! E uses fx
    // the first caller runs f; concurrent and later callers wait for and share its
    // result. If the running caller is cancelled before f finishes, the next
    // waiter runs f instead. A failed result is shared too; use a new Once to retry.
    // A fault in f is shared with every waiter as well.

pub fn semaphore(permits: Int) -> Semaphore  // Semaphore is a handle: copies share permits
fn (s: Semaphore) with[R, E, fx: Effects](f: fn() -> R ! E uses fx) -> R ! E uses fx
    // waits for a permit (cancellation point), runs f, releases the permit
```

When `f` cannot fail, `E` is `Never` and the `Failed` variant can be left out of
a `match` (core 2.3).

---

## 8. std/testing

`os`, `expect`, and `testing` are predeclared in test blocks; `expect` and
`testing` are also available anywhere in `_test.xo` files (core 9.2).

```
struct TestOs {
  net: FakeNet
  fs: MemFs
  clock: FakeClock
  rand: Rand          // seeded from the test seed
  env: FakeEnv
  stdio: FakeStdio
  proc: FakeProc
  tasks: task.Group   // lives for the test; cancelled when the test ends
}
```

Each fake satisfies its capability interface and adds control methods.

### 8.1 FakeNet

```
fn (n: FakeNet) handle(pattern: Str, h: fn(http.Request) -> http.Response)
    // pattern: "[METHOD ]<absolute url pattern>", e.g. "GET https://api.test/users/{id}"
fn (n: FakeNet) fail(pattern: Str, err: http.ClientErr)       // every matching request, until clear_failures
fn (n: FakeNet) fail_next(pattern: Str, err: http.ClientErr)  // the next matching request only
fn (n: FakeNet) clear_failures()
fn (n: FakeNet) requests() -> List[http.Request]     // every request seen, including failed ones
```

- `http.client(os.net)` calls registered handlers in process. Unhandled
  requests fail with `ClientErr.Connect("no fake handler for GET ...")`.
- `os.net.listen(":0")` gives an in memory listener, so a real `http.server`
  can be tested end to end with `http.client(os.net)`.

### 8.2 FakeClock

Virtual time starting at `2026-01-01T00:00:00Z`. When every task in the test is
blocked, time jumps to the next timer. Timers due at the same instant fire in
the order they were scheduled. Time does not move while cancelled tasks
unwind. `advance(d)` fires every timer due within `d`, in order, and lets the
woken tasks run before returning.

```
fn (c: FakeClock) advance(d: Duration)
fn (c: FakeClock) set(t: Time)
fn (c: FakeClock) sleeps() -> List[Duration]          // every sleep requested, in order
```

### 8.3 MemFs

In memory, case sensitive by default. `write` creates missing parent
directories. `write` sets the modified time to the
virtual clock's now; `rename` keeps the source's modified time. Paths in
`fail_next` and `files` are relative to the test's `os.fs` root; `fail_next`
matches an operation if any of its path arguments equals the given path
(for `rename`, either the source or the target).

```
enum FsOp { Read, Write, Rename, Remove, CreateDir, ReadDir }
fn (f: MemFs) fail_next(op: FsOp, path: Str, err: FsErr)   // next matching op fails once; `*` matches any run of characters within a segment
fn (f: MemFs) set_case_sensitive(on: Bool)
fn (f: MemFs) files() -> Map[Str, Bytes]                  // every file, by path
```

### 8.4 FakeEnv, FakeStdio, FakeProc

```
fn (e: FakeEnv) set(name: Str, value: Str)
fn (e: FakeEnv) set_args(args: List[Str])
fn (s: FakeStdio) set_input(text: Str)
fn (s: FakeStdio) output() -> Str                     // everything printed to stdout
fn (s: FakeStdio) errors() -> Str                     // everything printed to stderr
fn (p: FakeProc) exit_code() -> Int?                  // set when exit was called
fn (p: FakeProc) signal_after(d: Duration)            // next run_main gets SIGTERM after d (virtual time)
fn (p: FakeProc) on_run(cmd: Str, out: ProcOutput)
```

In a test, `os.proc.exit(code)` records the code and unwinds the code under
test back to the test block, which continues.

```
pub fn run_main[fx: Effects](main: fn(Os) -> () ! Error uses fx, os: TestOs, args: List[Str] = []) -> Int
    uses fx
    // runs a program's main against fakes and returns its exit status
    // (0, 1 for an error returned from main, or the code passed to exit)
```

Std enums are named with their module (`testing.FsOp.Rename`); the prefix may
be omitted when the expected type is known (`fail_next(Rename, ...)`).

### 8.5 Generators

Property tests (`test "x" for v: T`) generate values for any type built from
builtin types, structs, and enums. `testing.gen[T](seed: Int) -> T` produces
one value directly.

---

## 9. std/path

Pure helpers for `/` separated paths, the form every `Fs` capability uses on
every OS (section 3.1).

```
pub fn join(a: Str, b: Str) -> Str   // joins and cleans
pub fn dir(p: Str) -> Str            // all but the last element, "." if none
pub fn base(p: Str) -> Str           // the last element
pub fn ext(p: Str) -> Str            // extension with the dot, or ""
pub fn clean(p: Str) -> Str          // removes `.`, `..`, repeated `/`
```
