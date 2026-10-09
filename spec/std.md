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
NaN sorts after every other value (all NaNs are equal in the order) and
`-0.0` and `0.0` are equal: `-Inf < ... < -0.0 = 0.0 < ... < +Inf < NaN`
(decision 0053). This order drives `<`, `<=`, `>`, `>=` on Float (core
2.1), `sort` and `sort_by` (stable, so `-0.0` and `0.0` keep their input
order and NaNs end up last), derived `Ord` on types with Float fields,
`T: Ord` generic code, and `min`, `max`, `clamp`. `==` on Float is IEEE
(NaN differs from itself), not this order.

`Int`, the sized integers, `Float`, `Bool`, `Rune`, `Str`, `Duration`, `Time`,
and every std error type implement `Display`. `Bytes` does not (use
`to_str()`). `Json` is implemented by `derive Json` and by
builtin types (section 5). `SqlRow` is implemented only by `derive SqlRow`
(section 10.4).

Every `Ord` type has `a.min(b)`, `a.max(b)`, `a.clamp(lo, hi)`. `a.min(b)`
is `b` when `b < a` and otherwise `a`; `a.max(b)` is `b` when `b > a` and
otherwise `a`: on a tie the receiver is kept (`(-0.0).min(0.0)` is
`-0.0`, `(0.0).min(-0.0)` is `0.0`). Under the Float order `x.min(NaN)` is
`x` and `x.max(NaN)` is NaN. `a.clamp(lo, hi)` is `a.max(lo).min(hi)`.

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

A conversion is a call on the type name, `Float(n)`, not an associated
function: there is no `Float.from(n)`. XO0601 for an unknown associated
function of a numeric type called with one numeric argument (`Float.from(n)`,
`Int.from(x)`, `Int64.new(n)`) carries the note `convert with Float(x)` and,
as its first fix, the edit to `Float(n)`.

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
Duration.from_secs(s: Float) -> Duration      // rounds to the nearest nanosecond
Duration.from_secs_int(s: Int) -> Duration    // the same as `1s * s`; faults on overflow
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
fn (m: Mutex[T]) with_read[R, E, fx: Effects](f: fn(T) -> R ! E uses fx) -> R ! E uses fx

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

`Mutex.with_read` (decision 0063) is shared read only access: many readers
or one writer. `f` gets the value read only (mutating it is an error, and
its parameter cannot be declared `var`); nothing is written back. Readers
run at the same time as each other; `with` waits until no reader is
inside, and readers wait while a writer is inside.

- Writers are preferred: while a `with` waits, a new `with_read` waits
  behind it, so a stream of readers cannot starve a writer. So a reader
  must not wait (on a channel, a task, or another lock) for a reader that
  has not entered yet: with a writer queued in between that is a deadlock
  (XO-F001, reported with the readers that hold the mutex).
- In a real run the order among waiters is not fixed (as for `with`,
  decision 0047). Under `xo test` and `--seed` it is first come, first
  served, with the readers at the head of the queue entering together;
  readers inside together interleave at scheduling points like any tasks,
  and `--explore` enumerates those interleavings.
- Waiting is a cancellation point; a canceled waiter leaves the queue
  (readers queued behind a canceled writer enter at once).
- Calling `with` or `with_read` on a mutex the task already holds in
  either mode faults (`XO-F004`).
- A value read in `with_read` and used in a later `with` on the same mutex
  gets the split critical section warning `XO0804` (core 5.4).

```
struct Stats {
  hits: Int
  last: Str
}

fn record(m: Mutex[Stats], path: Str) {
  m.with(fn(var s) {
    s.hits += 1
    s.last = path
  })
}

fn report(m: Mutex[Stats]) -> Str {
  m.with_read(fn(s) { "${s.hits} hits, last ${s.last}" })
}
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
fn (l: Listener) addr() -> Str                     // "host:port" actually bound
fn (l: Listener) accept() -> Conn ! NetErr         uses net.listen
fn (l: Listener) close()
fn (c: Conn) read(max: Int) -> Bytes? ! NetErr     uses net   // None at the end of the stream
fn (c: Conn) write(data: Bytes) -> () ! NetErr     uses net   // writes every byte
fn (c: Conn) close()
```

TCP connections (decision 0071): `accept` waits for the next connection;
`read` returns at most `max` bytes (at most 1 MiB; fewer when fewer are
there) and `None` at the end of the stream; `write` returns when every
byte is written. All three are cancellation points that report
cancellation as `Canceled` (core 7.2). `close` is idempotent and makes a
waiting `accept`, `read`, or `write` on the same listener or connection
fail with `Io`. A refused `dial` is `Refused`.

`addr` reports the port the system chose for `listen(":0")`. An unspecified
host (`:0`, `0.0.0.0`, `[::]`) is reported as the loopback address
(`127.0.0.1`, `::1`), so the result can be dialed locally:
`"http://${l.addr()}/health"`. The fake network of tests gives
`127.0.0.1:49152`, `127.0.0.1:49153`, ... for `:0`. Under record and
replay (core 11.3) the recorded address is returned (decision 0055).

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
fn ([var] r: Router) wrap[E: Error, fx: Effects](m: fn(Request, Next) -> Response ! E uses fx) uses fx
fn ([var] r: Router) mount(prefix: Str, sub: Router)   // sub's routes under prefix, behind sub's middleware
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
- Middleware may fail: `Err(e)` from a middleware is mapped to a response
  exactly like a handler's (section 4.2), and the middleware registered
  before it see that response. A middleware that cannot fail has
  `E = Never`. `Next` itself cannot fail: it returns the mapped response
  of everything inside.
- Scoped middleware: `r.mount(prefix, sub)` adds every route of `sub` to
  `r` with `prefix` (a path, segments as in patterns) in front of its
  pattern. A mounted route runs `r`'s middleware, then `sub`'s, then the
  handler. A request no route of `sub` matches never reaches `sub`'s
  middleware (it is a 404 or 405 of `r`). The routes are copied (`Router`
  is a value): registering on `sub` after `mount` does not change `r`. To
  guard one route, mount a router holding only that route. `mount` has no
  effects of its own; they were counted where `sub` registered them.
- `Router`, `Next`, `call`, and `serve` carry no effects in their types:
  effects are accounted where handlers and middleware are registered, which
  is where their capabilities are captured.

```
// public routes on r, authenticated routes on api, no path comparison
fn routes(db: sql.Db) -> http.Router uses net.dial {
  var api = http.router()
  api.wrap(fn(req, next) -> http.Response ! AuthErr {
    let user = authenticate(db, req)?
    next(req.with_value(user_key(), user))
  })
  api.handle("GET /notes", fn(req) { list(db, req.value(user_key()).or_fault("set by auth")) })
  var r = http.router()
  r.handle("GET /health", fn(_req) { http.text(200, "ok") })
  r.mount("/api", api)
  r
}
```

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
fn (r: Request) with_header(name: Str, value: Str) -> Request

struct Key[T] {}                                   // handle
pub fn key[T](name: Str) -> Key[T]
fn (k: Key[T]) name() -> Str
fn (r: Request) with_value[T](k: Key[T], v: T) -> Request
fn (r: Request) value[T](k: Key[T]) -> T?

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
fn (r: Response) text() -> Str?                     // the body as text; None if not valid UTF-8
fn (r: Response) decode_json[T: Json]() -> T ! json.DecodeErr
```

`text` and `decode_json` read the whole body, collecting a streamed one
first; `text` is `r.body.to_str()` for a body that is not streamed. In
tests: `expect(resp.text()) == Some("ok")`, or `resp.text() ?? ""`.

Typed request values (decision 0055) let a middleware hand a value to the
handlers behind it without encoding it in a header. `with_value` returns a
copy of the request carrying `v` under `k`; `value` returns the latest value
set under `k`, or `None`. A key is identified by its name and its type
`T`: `http.key[User]("user")` called twice gives the same key, and
`http.key[Int]("user")` is a different one, so a value is always read back
at the type it was stored with. Values are local to the process: they are
not headers, are never sent by a client or read from the network, take no
part in `==`, `hash`, or `debug` of a `Request`, and a fresh request
(`new_request`, or one received by `serve`) has none. So a client cannot
forge one. Declare a key once as a function or pass it around:

```
fn user_key() -> http.Key[User] { http.key("user") }

fn me(req: http.Request) -> http.Response ! ApiErr {
  let user = req.value(user_key()).or_err(ApiErr.Unauthorized)?
  http.json(200, user)
}
```

`with_header` returns a copy with the header set (canonical key), like
`Response.with_header`.

### 4.4 Server

```
pub fn server(l: Listener, r: Router) -> Server
fn (s: Server) serve() -> () ! HttpErr uses net.listen
fn (s: Server) addr() -> Str                       // the listener's addr()
enum HttpErr { Io(msg: Str), Canceled }
```

`serve` runs until its scope is cancelled, then stops accepting, waits for
in flight requests, and returns `Ok(())`. Each request runs as an isolated
task with its own span and request id. Its memory may come from a request
scoped arena (core 5.5, decision 0066), which no program can observe. To run a server alongside background
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
struct Record { level: Level, msg: Str, fields: Map[Str, Str] }   // as captured in tests (8.6)
```

```
log.info("listening", {"addr": addr, "port": port})
```

Every record includes the time, the current span, and the request id.
Without `configure`, logs go to standard error as text at `Info`.

Under `xo test` (decision 0057) each test has its own log sink, starting at
`Info` with no output configured. Records that would go to standard error
are buffered instead and printed under the test's result only when the
test fails (or with `xo test -v`), like `t.Log` in Go; `--json` adds them
as a `log` array to the test's line. `log.configure(os.stdio, ...)` in a
test (or in a `main` run by `testing.run_main`) sends records to the fake
standard error instead, as in a program. Either way every record that
passes the level is also kept for `testing.logs()` (section 8.6). Nothing
logged in a test reaches the real standard error.

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
  can be tested end to end with `http.client(os.net)`. Its `addr()` is
  `127.0.0.1:49152`, then `:49153`, ... A request to a listener whose
  `serve` has not started yet waits until it starts, like a connection in
  a real socket's backlog (a test that never serves it reports a deadlock
  naming the wait); after `serve` returns, requests fail with
  `ClientErr.Connect`.

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

### 8.6 Captured logs

```
pub fn logs() -> List[log.Record]     // records logged so far in this test, oldest first
```

A record's `fields` hold each value's text form, in the order given. Only
records at or above the sink's level are kept (`Info` unless the test
calls `log.configure`). Each test, and each case of a property test,
starts with none; tasks of the test and a `main` run by `run_main` log to
the same list. Outside a test, `logs()` is empty.

```
test "a failed purge is logged" {
  purge_once(broken_db(), os.clock)
  let recs = testing.logs()
  expect(recs.map(fn(r) { r.msg })) == ["purge failed"]
  expect(recs[0].level) == log.Level.Warn
}
```

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

---

## 10. std/sql

Status: draft, implemented with an in memory driver only (2026-10-08). Real
drivers (Postgres, SQLite, ...) are Go packages bound with `use go` and
registered with `rt.RegisterSqlDriver`; none ships yet.

### 10.1 Opening

```
struct Db {}                                 // handle: copies share the database

pub fn open(net: Net, driver: Str, dsn: Str,
            max_open: Int = 10, max_idle: Int = 2, max_lifetime: Duration = 30m)
    -> Db ! SqlErr uses net.dial
pub fn memory(schema: Str = "") -> Db ! SqlErr         // no capability, no effect
// std/testing: fake_db(schema: Str) -> Db ! SqlErr    // same as memory(schema)
```

A network driver is opened from the `Net` capability, so a function that is not
given a `Net` (or an already opened `Db`) cannot reach a database. The in
memory driver performs no I/O and needs no capability; it is what tests use.
`schema` is a `;` separated list of statements run on the fresh database.

Pooling: `max_open` bounds the operations and open transactions in flight on a
`Db`; a task over the limit waits (a cancellation point, it gets `Canceled`).
`max_idle` and `max_lifetime` are passed to the driver (the in memory driver
ignores them). `max_open` below 1 is `Param`.

Every operation on a `Db` or `Tx` declares `uses net.dial`, because the `Db`
may be network backed. Declaring more than the in memory driver performs is
allowed (core 6.2), so the same code runs against both.

### 10.2 Parameters and values

```
enum SqlValue { Null, Integer(n: Int), Text(s: Str), Real(f: Float), Flag(b: Bool), At(t: Time) }
pub fn null() -> SqlValue
pub fn int(n: Int) -> SqlValue       // also str, float, bool, time
```

Parameters are written `$1`, `$2`, ... and passed as a `List[SqlValue]`
(`[sql.str(name), sql.int(age)]`). Decision: a closed enum rather than
`List[Display]`, because the driver needs the type of each value (an `Int` and
its text look the same through `Display`), and the five variants cover the
column types every driver has. The count must equal the highest placeholder
number, else `Param`. Values are never interpolated into the statement by the
library; build the statement text from constants.

### 10.3 Queries

```
fn (d: Db) query(q: Str, args: List[SqlValue] = []) -> List[Row] ! SqlErr uses net.dial
fn (d: Db) query_one(q: Str, args: List[SqlValue] = []) -> Row ! SqlErr uses net.dial   // NoRows if empty; extra rows ignored
fn (d: Db) exec(q: Str, args: List[SqlValue] = []) -> ExecResult ! SqlErr uses net.dial
fn (d: Db) ping() -> () ! SqlErr uses net.dial
fn (d: Db) close() -> () ! SqlErr               // later operations fail with Closed; idempotent

struct ExecResult { rows_affected: Int, last_insert_id: Int }   // id is 0 when the driver has none
```

`query` runs a statement that returns rows, `exec` one that does not. A
write with a `RETURNING` clause returns rows, so it goes through `query`
(or `query_one`); through `exec` its rows are dropped.

### 10.4 Rows

```
struct Row {}
fn (r: Row) columns() -> List[Str]
fn (r: Row) is_null(name: Str) -> Bool ! SqlErr
fn (r: Row) int(name: Str) -> Int ! SqlErr       // also str, float, bool, time
fn (r: Row) int_opt(name: Str) -> Int? ! SqlErr  // also str_opt, float_opt, bool_opt, time_opt
```

Accessors read by column name (the first column of that name). A missing
column is `NoColumn`; a value of another type, or NULL read by a non optional
accessor, is `Mismatch(column, msg)`. `float` also accepts an integer. The
`_opt` forms map NULL to `None`.

Rows into structs (decision 0061):

```
interface SqlRow {}                              // prelude marker; only `derive SqlRow` implements it
fn (r: Row) decode[T: SqlRow]() -> T ! SqlErr
fn (d: Db) query_as[T: SqlRow](q: Str, args: List[SqlValue] = []) -> List[T] ! SqlErr uses net.dial
fn (d: Db) query_one_as[T: SqlRow](q: Str, args: List[SqlValue] = []) -> T ! SqlErr uses net.dial
fn (t: Tx) query_as[T: SqlRow](q: Str, args: List[SqlValue] = []) -> List[T] ! SqlErr uses net.dial
fn (t: Tx) query_one_as[T: SqlRow](q: Str, args: List[SqlValue] = []) -> T ! SqlErr uses net.dial
```

`derive SqlRow` is allowed on a struct without type parameters whose fields
are `Int`, `Float`, `Str`, `Bool`, `Time`, or an `Option` of one of them
(else XO0404 at the derive). `decode` reads each field from the column of
the same name with the rules of the accessors above: a missing column is
`NoColumn(name)`, a value of another type is `Mismatch(column, msg)`, NULL is
`None` for an `Option` field and `Mismatch` for any other. Fields are read in
declaration order and the first failure is returned. Columns without a field
are ignored, so `SELECT *` works with a struct naming some of the columns.
Field defaults do not apply: every field needs its column. `query_as` is
`query` followed by `decode` of every row; `query_one_as` is `query_one`
followed by `decode` (`NoRows` when there is no row).

```
struct Note {
  id: Int
  owner: Int
  body: Str
  expires: Time
  tag: Str?
} derive SqlRow, Json

fn notes(db: sql.Db, owner: Int) -> List[Note] ! sql.SqlErr uses net.dial {
  db.query_as[Note]("SELECT * FROM notes WHERE owner = $1 ORDER BY id", [sql.int(owner)])
}

fn add(db: sql.Db, owner: Int, body: Str, at: Time) -> Note ! sql.SqlErr uses net.dial {
  db.query_one_as[Note]("INSERT INTO notes (owner, body, expires) VALUES ($1, $2, $3) RETURNING *",
    [sql.int(owner), sql.str(body), sql.time(at)])
}
```

### 10.5 Transactions

```
fn (d: Db) begin() -> Tx ! SqlErr uses net.dial
fn (t: Tx) query(q: Str, args: List[SqlValue] = []) -> List[Row] ! SqlErr uses net.dial
fn (t: Tx) query_one(q: Str, args: List[SqlValue] = []) -> Row ! SqlErr uses net.dial
fn (t: Tx) exec(q: Str, args: List[SqlValue] = []) -> ExecResult ! SqlErr uses net.dial
fn (t: Tx) commit() -> () ! SqlErr uses net.dial
fn (t: Tx) rollback() -> () ! SqlErr uses net.dial
```

A transaction holds one pool permit from `begin` until `commit` or
`rollback`; a driver that is a single writer (the in memory one) also allows
one open transaction at a time, later `begin` calls wait. Commit a second time,
or use a finished `Tx`, and you get `TxDone`. `rollback` on a finished `Tx`
does nothing and succeeds, so the cleanup pattern is:

```
let tx = db.begin()?
defer { tx.rollback().ignore_err("nothing to undo if the commit ran") }
tx.exec(...)?
tx.commit()?
```

`defer` blocks are shielded (core 3.5), so the rollback still runs when the
task is cancelled or the function returns an error. A statement that fails
inside a transaction leaves the transaction usable and its earlier work intact.
A `Tx` is a handle: copies share the transaction.

### 10.6 Errors and the in memory driver

```
enum SqlErr {
  Syntax(msg: Str), Constraint(msg: Str), Param(msg: Str), NoTable(name: Str),
  NoRows, NoColumn(name: Str), Mismatch(column: Str, msg: Str),
  Conn(msg: Str), Driver(msg: Str), TxDone, Closed, Canceled
} derive Display
```

`Conn` is a lost or refused connection, `Driver` any other driver failure
(including an unknown driver name), `Constraint` a key, uniqueness, NOT NULL,
or type violation.

The in memory driver (`memory`, `testing.fake_db`) supports: `CREATE TABLE [IF
NOT EXISTS]` (types `INT`/`INTEGER`/`BIGINT`, `TEXT`/`VARCHAR`, `REAL`/`FLOAT`,
`BOOL`, `TIMESTAMP`, `SERIAL`; `PRIMARY KEY` on a single integer column
auto-assigns keys; `NOT NULL`, `UNIQUE`, `DEFAULT literal`), `DROP TABLE`,
`INSERT ... VALUES` (several tuples), `SELECT` of `*`, expressions with `AS`,
or `COUNT(*)` from one table with `WHERE`, `ORDER BY` (several keys, `ASC`/`DESC`),
`LIMIT`/`OFFSET`, `UPDATE ... SET ... WHERE`, and `DELETE ... WHERE`.
`INSERT`, `UPDATE`, and `DELETE` take `RETURNING` with the items of
`SELECT` (`*`, expressions, `AS`; not `COUNT(*)`), yielding the inserted
rows, the updated rows with their new values, or the deleted rows, in table
order; generated keys and defaults are filled in.
Expressions: columns, literals, `$n`, `+ - * /`, `= != <> < <= > >=`, `AND`,
`OR`, `NOT`, `IS [NOT] NULL`, `[NOT] IN (...)`, `[NOT] LIKE`. No joins,
grouping, or subqueries. Reads inside a transaction see its own
writes; reads outside see the last commit. A write through `query` that
fails changes nothing, like one through `exec`.
Results of a `Db` are not recorded by record and replay (core 11.3) yet.

---

## 11. std/crypto

Status: draft (2026-10-08, decisions 0056 and 0059). Go standard library
underneath, plus Argon2id and BLAKE2b vendored from `golang.org/x/crypto`
into the runtime (no module dependency).

```
pub fn sha256(data: Bytes) -> Bytes                     // 32 bytes
pub fn hmac_sha256(key: Bytes, msg: Bytes) -> Bytes     // 32 bytes
pub fn equal(a: Bytes, b: Bytes) -> Bool                // constant time in the contents
pub fn random_bytes(r: Rand, n: Int) -> Bytes uses rand // faults when n < 0
pub fn token(r: Rand, n: Int = 32) -> Str uses rand     // n random bytes, unpadded URL safe base64
pub fn hash_password_argon2id(r: Rand, password: Str, memory_kib: Int = 19456,
                              iterations: Int = 2, parallelism: Int = 1) -> Str uses rand
pub fn hash_password(r: Rand, password: Str, iterations: Int = 600000) -> Str uses rand
pub fn verify_password(password: Str, hash: Str) -> Bool
```

- Hashing and comparison are pure: no capability, no effect.
- Randomness comes only from the `Rand` capability (section 3.4), as
  everywhere else: cryptographically secure in programs, seeded from the
  test seed in tests, so a test that creates tokens or salts is
  reproducible. A function that makes tokens takes a `Rand` and declares
  `uses rand`.
- `equal` takes time that depends on the lengths but not on the contents;
  compare digests of equal length (for example two `sha256` results) when
  the length is secret too.
- `hash_password_argon2id` is the recommended password hash: Argon2id
  (RFC 9106, version 0x13) with a 16 byte salt from `r` and a 32 byte key,
  encoded in the PHC string format
  `$argon2id$v=19$m=<memory_kib>,t=<iterations>,p=<parallelism>$<salt>$<key>`
  (salt and key in unpadded standard base64), which other Argon2 libraries
  read. The defaults, 19456 KiB (19 MiB), 2 passes, 1 lane, are the OWASP
  minimum for Argon2id (Password Storage Cheat Sheet); raise `memory_kib`
  first when the host allows. One hash at the defaults takes about 20 ms
  on an Apple M series core. It faults when `parallelism` is outside
  `1..=255`, `memory_kib` outside `8 * parallelism..=4194304` (4 GiB), or
  `iterations` outside `1..=1000`. Tests may pass small values
  (`memory_kib: 64, iterations: 1`).
- `hash_password` is PBKDF2-HMAC-SHA256 with a 16 byte salt from `r` and a
  32 byte key, encoded as `pbkdf2-sha256$<iterations>$<salt>$<key>`
  (salt and key in unpadded standard base64). It faults when `iterations`
  is outside `1..=100000000`. The default, 600000, follows the OWASP 2023
  guidance for PBKDF2-HMAC-SHA256; tests may pass a small count.
  PBKDF2 stays supported for existing hashes and for hosts that need a
  FIPS approved function; it is not memory hard, so prefer Argon2id for new
  code.
- `verify_password` tells the formats apart by prefix (`$argon2id$` or
  `pbkdf2-sha256$`), recomputes the key with the stored salt and
  parameters, and compares in constant time. A malformed hash, another
  Argon2 variant or version, or parameters outside the bounds above is
  `false`, never an error. To move users to Argon2id, rehash on the next
  successful login when the stored hash starts with `pbkdf2-sha256$`.

Storing API tokens: tokens from `token` have 256 bits of entropy, so a
plain `sha256` digest is enough (no salt or stretching needed) and can be
looked up by value; compare the digest found with `equal`:

```
fn issue(db: sql.Db, r: Rand, user: Int) -> Str ! sql.SqlErr uses (rand, net.dial) {
  let t = crypto.token(r)
  db.exec("INSERT INTO tokens (user_id, digest) VALUES ($1, $2)",
    [sql.int(user), sql.str(crypto.sha256(t.bytes()).to_hex())])?
  t
}
```

Passwords chosen by people have little entropy: store
`hash_password_argon2id(os.rand, pw)` and check with `verify_password`.
