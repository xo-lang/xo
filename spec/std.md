# Xo Standard Library and Prelude

Status: draft 0.8 (2026-10-09; the same draft number as `spec/core.md`, decision 0089)
Companion to `spec/core.md`.

Signatures are written in Xo syntax. `[var]` before a receiver means the method
mutates and needs a `var` binding (core 3.4). Effects are listed with `uses`.
Everything here joins the compatibility promise with the first frozen
edition, named at 1.0 (core 13); until then the only edition is `preview`
(decision 0155), which may still change.

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
`Eq` and `Hash` are automatic when all fields implement them (handles and
function values never do, so a value holding one is not comparable: core
2.9, decision 0112); `Ord` comes from
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
and every std error type implement `Display`, and so does every type with
`derive Display`: `e.display()` is callable on each of them (the same text
as `"${e}"`), as well as through a `T: Display` bound or an `Error` value.
`Bytes` does not (use `to_str()`). `Json` is implemented by `derive Json` and by
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
Int(r: Rune) -> Int                  // the code point; every integer type (core 2.1)
Rune(n: Int) -> Rune                 // faults unless n is a Unicode scalar value
Rune.try(n: Int) -> Rune?

fn (n: Int) abs() -> Int
fn (n: Int) pow(e: Int) -> Int
fn (n: Int) wrapping_add(m: Int) -> Int      // also wrapping_sub, wrapping_mul
fn (n: Int) saturating_add(m: Int) -> Int    // also saturating_sub, saturating_mul
fn (n: Int) checked_add(m: Int) -> Int?      // also checked_sub, checked_mul, checked_div

fn (f: Float) floor() -> Float               // also ceil, round, trunc, abs, sqrt
fn (f: Float) is_nan() -> Bool
```

`Int.parse(s)` is Go's `strconv.ParseInt(s, 10, 64)` and `Float.parse(s)`
is `strconv.ParseFloat(s, 64)`, None where they return an error: decimal
and hexadecimal (`0x1.8p1`) text, underscores between digits (`1_000`),
`inf`, `infinity` (either sign), and `nan`, any case; None when the
value is out of range. The text of a Float (interpolation, Display,
Debug) is the shortest that reads back the same value
(`strconv.FormatFloat(f, 'g', -1, 64)`) with `.0` added to whole
numbers: `1.0`, `0.1`, `1e+21`, `-0.0`, `+Inf`, `NaN`.

A conversion is a call on the type name, `Float(n)`, not an associated
function: there is no `Float.from(n)`. XO0601 for an unknown associated
function of a numeric type called with one numeric argument (`Float.from(n)`,
`Int.from(x)`, `Int64.new(n)`) carries the note `convert with Float(x)` and,
as its first fix, the edit to `Float(n)`.

### 2.2 Str, Rune, Bytes

`Str` is UTF-8. `+` concatenates `Str` and `Bytes`. `Bytes` holds any bytes;
build one piece by piece with `BytesBuilder` (2.7), whose appends are
amortized O(1) where `out = out + more` copies `out` each time.

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

fn (b: Bytes) len() -> Int                   // b[i] is the byte at i, a UInt8
fn (b: Bytes) is_empty() -> Bool             // `for x in b` yields each byte (core 3.5)
fn (b: Bytes) to_str() -> Str?               // None if not valid UTF-8
fn (b: Bytes) to_hex() -> Str                // lowercase
fn (b: Bytes) to_base64() -> Str             // standard encoding, padded
Bytes.from_hex(s: Str) -> Bytes?
Bytes.from_base64(s: Str) -> Bytes?
fn (b: Bytes) rune_count() -> Int            // an invalid byte counts as one rune
fn (b: Bytes) decode_rune(i: Int) -> (Rune, Int)  // the rune at byte i and its size
fn (b: Bytes) quote() -> Str                 // also on Str: a quoted literal

fn (r: Rune) is_digit() -> Bool              // also is_letter, is_space, to_lower, to_upper
fn (r: Rune) is_print() -> Bool
fn (r: Rune) quote() -> Str                  // 'a', '\n', '\u2028'
```

Unicode and quoting follow Go's `unicode`, `unicode/utf8`, and `strconv`
packages exactly (decision 0079), so text a Go program prints can be
reproduced:

- `is_letter` is a letter (category L), `is_digit` a decimal digit (Nd),
  `is_space` white space (`\t \n \v \f \r`, space, U+0085, U+00A0, and
  category Zs, plus U+2028 and U+2029), `is_print` `strconv.IsPrint` (a
  letter, mark, number, punctuation, symbol, or the ASCII space).
- `decode_rune(i)` reads the UTF-8 sequence at byte `i`: `(U+FFFD, 1)`
  for an invalid or short sequence, `(U+FFFD, 0)` at `i == len()`, and a
  fault when `i` is negative or past the end.
- `r.quote()` is `strconv.QuoteRune`: single quotes, `\'` and `\\`
  escaped, printable runes as they are, `\a \b \f \n \r \t \v`, then
  `\xHH` below U+0080, `\uHHHH`, or `\UHHHHHHHH`.
- `s.quote()` and `b.quote()` are `strconv.Quote`: double quotes, the same
  escapes with `\"` in place of `\'`, and every byte of an invalid UTF-8
  sequence as `\xHH`.

Slicing (`s[a..b]`) is in core 3.6.

### 2.3 List[T]

```
List[T].with_capacity(n: Int) -> List[T]     // empty, room for n elements
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

`List[T].with_capacity(n)` (also `List.with_capacity(n)` where the type is
known) is an empty list, equal to `[]`, whose first `n` pushes append
without growing it; `n` below 0 counts as 0. Capacity is not observable
(decision 0170).

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
Duration.parse(s: Str) -> Duration ! DurationErr
fn (d: Duration) seconds() -> Float
fn (d: Duration) millis() -> Int             // truncated
fn (d: Duration) nanos() -> Int
fn (d: Duration) scale(f: Float) -> Duration // rounds to the nearest nanosecond
fn (d: Duration) ceil_seconds() -> Int
fn (d: Duration) abs() -> Duration           // durations may be negative

struct DurationErr { input: Str }  // displays as `invalid duration "1x": expected numbers with units ...`

Time.UNIX_EPOCH: Time
Time.from_unix(secs: Int) -> Time
Time.from_unix_millis(ms: Int) -> Time
Time.parse_rfc3339(s: Str) -> Time?
fn (t: Time) unix() -> Int
fn (t: Time) unix_millis() -> Int
fn (t: Time) format_rfc3339() -> Str
```

`Duration.parse` accepts Go's `time.ParseDuration` syntax after trimming
white space: an optional sign, then one or more decimal numbers (with an
optional fraction) each followed by a unit `ns`, `us` (or `µs`), `ms`,
`s`, `m`, `h`: `"1.5s"`, `"-2h45m"`, `"300ms"`. `"0"` alone is zero. A
number without a unit, an unknown unit, or a value past about 292 years
is `Err(DurationErr{input: s})`. It is the form JSON decoding of a
`Duration` accepts, and it reads back what `Duration` displays (decision
0088). Unlike `Int.parse`, it returns a `Result`: the error says what a
duration looks like.

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
fn (b: Buf[T]) len() -> Int
fn (b: Buf[T]) freeze() -> List[T]           // moves b
StrBuilder.new() -> StrBuilder
fn ([var] b: StrBuilder) write(s: Str)
fn (b: StrBuilder) freeze() -> Str           // moves b
BytesBuilder.new() -> BytesBuilder           // also with_capacity(n)
fn ([var] b: BytesBuilder) write(data: Bytes)
fn ([var] b: BytesBuilder) write_byte(x: UInt8)
fn ([var] b: BytesBuilder) write_str(s: Str)  // its UTF-8 bytes
fn ([var] b: BytesBuilder) write_rune(r: Rune) // its UTF-8 encoding
fn (b: BytesBuilder) len() -> Int            // also on StrBuilder
fn (b: BytesBuilder) freeze() -> Bytes       // moves b

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

On Windows (decision 0197, stage 4) `os.fs`, `sub`, and the other
operations also take the forms the OS writes, so a path read from a
variable or an argument works as it is: `\` separates as `/` does; a
drive path (`C:/data`, `C:\data`) and a UNC path (`//server/share/x`,
`\\server\share\x`) are absolute; a path rooted at `/` without a drive
is on the working directory's drive. Every path std gives back uses `/`
only: `FileInfo.path`, `create_temp`, `read_link`, `glob`, and
`Env.program`, `home`, `config_dir`, and `cache_dir` (3.5). On other OSes
`\` is an ordinary character of a name, as before.

`cwd_path` (decision 0218) is the working directory as an absolute path
with `/` separators, the directory `cwd()` is rooted at, asked of the OS
(getcwd; `GetCurrentDirectoryW` on Windows) and never taken from `$PWD`
alone: `$PWD` is used only when it names the same directory (it keeps
the links the shell's path has, as Go's `os.Getwd` does), so a stale
`$PWD` from a parent that started the program without a shell (`PWD=/`
from the macOS Dock) is ignored. It is the same on every capability
derived from `os.fs`. It fails with `Io` when the OS cannot give it
(a removed directory). WASI has no working directory: there it fails
with `Io` (planned: the wasm backend reports the call as unsupported
today, as it does `cwd()`).
A test's `MemFs` gives `/`, its root, where its `cwd()` is rooted.

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
fn (f: Fs) cwd_path() -> Str ! FsErr                         // the working directory's absolute path, from the OS
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

Handles, links, globs, and locks (decision 0163; the Go backend and
MemFs, and native builds on the real filesystem, decision 0172; a native
test binary's MemFs reports them as XO1201, planned):

```
fn (f: Fs) open(path: Str) -> File ! FsErr        uses fs.read   // read as a stream
fn (f: Fs) create(path: Str) -> File ! FsErr      uses fs.write  // write; created or truncated
fn (f: Fs) open_append(path: Str) -> File ! FsErr uses fs.write  // write at the end; created if missing
fn (f: Fs) append(path: Str, data: Bytes) -> () ! FsErr uses fs.write
fn (f: Fs) copy(from: Str, to: Str) -> () ! FsErr uses fs        // contents and permission bits
fn (f: Fs) mode(path: Str) -> Int ! FsErr         uses fs.read   // permission bits: 0o644
fn (f: Fs) set_mode(path: Str, mode: Int) -> () ! FsErr uses fs.write
fn (f: Fs) symlink(target: Str, link: Str) -> () ! FsErr uses fs.write
fn (f: Fs) read_link(path: Str) -> Str? ! FsErr   uses fs.read   // None when not a link
fn (f: Fs) glob(pattern: Str) -> List[Str] ! FsErr uses fs.read
fn (f: Fs) lock(path: Str) -> FileLock ! FsErr    uses fs.write  // waits; a cancellation point
fn (f: Fs) try_lock(path: Str) -> FileLock? ! FsErr uses fs.write

struct File {}                                    // handle
fn (f: File) read(max: Int) -> Bytes? ! FsErr     uses fs.read   // None at the end
fn (f: File) read_line() -> Str? ! FsErr          uses fs.read   // without \n or \r\n
fn (f: File) write(data: Bytes) -> () ! FsErr     uses fs.write
fn (f: File) close() -> () ! FsErr                               // closing again does nothing
struct FileLock {}                                // handle
fn (l: FileLock) unlock()                                        // unlocking again does nothing
```

```
fn count_errors(fs: Fs, path: Str) -> Int ! FsErr uses fs.read {
  let f = fs.open(path)?
  defer { f.close().ignore_err("read only") }
  var n = 0
  while let Some(line) = f.read_line()? {
    if line.contains("ERROR") {
      n += 1
    }
  }
  n
}
```

- A `File` is opened for reading (`open`) or writing (`create`,
  `open_append`); the other kind of operation is `Io(path, "file is open
  for reading")` (or writing). `read` returns at most `max` bytes (at
  most 1 MiB), `None` at the end. Operations after `close` are `Io(path,
  "file already closed")`. Writes are not buffered by Xo: each `write`
  reaches the OS. Handles are not recorded by record and replay yet.
- `open` of a directory is `IsDir`. `create` and `open_append` do not
  create missing parent directories on the real filesystem (MemFs does,
  as its `write` does).
- `copy` reads `from` and writes `to` (created or truncated) with
  `from`'s permission bits; a directory is `IsDir(from)`.
- `mode` and `set_mode` are the permission bits (`0o777` mask). Windows
  reports and sets what it can (read only or not).
- `symlink(target, link)` stores `target` as written; a relative target
  is relative to the link's directory. In a sub capability a target that
  is absolute or leaves the root (`..` past it) is `Denied(link)`: a sub
  capability is a lexical boundary, and links created through it must
  not escape it. Links that already exist are followed by the OS as
  before (the boundary is lexical, decision 0024).
- `glob(pattern)` lists the paths matching `pattern`, sorted, relative to
  the capability root as the pattern is written: `*` (any run of
  characters but `/`), `?`, `[abc]`, `[a-z]`, `[^a]` in a segment, and a
  segment `**` for any number of directories (zero included). Names that
  start with `.` match only a segment that starts with `.`. A missing
  directory matches nothing; a malformed pattern is `Io(pattern, "bad
  pattern: ...")`. It works through `read_dir`, so attenuation applies.
- `lock(path)` takes an exclusive advisory lock on `path` (created if
  missing), waiting until no other holder has it; waiting is a
  cancellation point (`Canceled`). `try_lock` returns `None` at once when
  the lock is held. The lock is the OS's (`flock` on Unix, `LockFileEx`
  on Windows), so it excludes other processes that lock the same file,
  and other `lock` calls in the same program. It is advisory: reads and
  writes are not blocked. A program that exits releases its locks.

### 3.2 Net (`uses net.dial` / `net.listen`)

```
enum NetErr { InvalidAddr(msg: Str), Refused, Timeout, Io(msg: Str), Canceled, Denied(addr: Str) }

fn (n: Net) listen(addr: Str) -> Listener ! NetErr uses net.listen
fn (n: Net) dial(addr: Str) -> Conn ! NetErr       uses net.dial
fn (n: Net) only(hosts: List[Str]) -> Net          // attenuation (below)
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

**Unix domain sockets** (decision 0190): an address `unix:PATH` listens
on or dials the socket file PATH (`os.net.listen("unix:/tmp/app.sock")`).
`addr` returns the address as given; closing the listener removes the
socket file (once), as Go's does; a listen on an existing file fails
with `Io` (`listen unix PATH: bind: address already in use`); a dial of
a missing file is `Io` (`dial unix PATH: connect: no such file or
directory`) and of a file nobody listens on is `Refused`. In an
attenuated `Net` an entry `unix:PATH` admits exactly that address (the
path keeps its case); host entries never admit a socket and `unix:`
entries never admit a host. FakeNet listens on them in memory. Used by `xo serve`.

Most code uses `std/http` instead of raw connections.

**Attenuation** (decision 0156): `n.only(hosts)` is a `Net` whose `dial`
and `listen` succeed only for an address whose host matches an entry:
an exact host or IP (`api.example.com`, `127.0.0.1`, compared without
case), `*.example.com` (any subdomain, not `example.com` itself), or
`host:port` (that host and port only). A listen address without a host
(`:8080`) has the host `0.0.0.0`. Any other address fails with
`Denied(addr)` before a name is looked up. An HTTP client made from it
(`http.client(n)`, 4.5) checks each request's host and port (80 or 443
by scheme when the URL has none), and each redirect, failing with
`ClientErr.Connect("denied by Net.only: <host:port>")`; `sql.open` with
an attenuated `Net` fails with `SqlErr.Conn` (the driver dials by
itself, so the list cannot be enforced). `only` on an attenuated `Net`
keeps both lists: an address must pass each. Names are checked as
written, not as resolved: an allowed name that resolves to another
address is allowed. Native builds (`--no-gc`) have it too (decision
0175): a `Net` there is the lists of its attenuations; hosts in dial and
listen addresses and in redirects are lowered ASCII only.

```
let api = os.net.only(["api.example.com"])
let weather = weather.client(api)   // the library can reach api.example.com only
```

### 3.3 Clock (`uses clock`)

```
fn (c: Clock) now() -> Time
enum ClockErr { Canceled }

fn (c: Clock) sleep(d: Duration)             // cancellation point; unwinds when cancelled
fn (c: Clock) sleep_or_cancel(d: Duration) -> () ! ClockErr
    // like sleep, but cancellation is Err(Canceled) instead of an unwind
fn (c: Clock) since(t: Time) -> Duration
```

`sleep_or_cancel` lets a function return cancellation as its own error
(core 7.2, decision 0088):

```
fn backoff(clock: Clock, d: Duration) -> () ! FetchErr uses clock {
  match clock.sleep_or_cancel(d) {
    Ok(_) => Ok(())
    Err(Canceled) => Err(FetchErr.Cancelled)
  }
}
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

Native builds (`--no-gc`, decision 0083) draw from the operating system's
generator (`getentropy`, `getrandom`); `shuffle` draws one `int` per
step as on the Go backend, so a test's seeded shuffle is the same
(decision 0107).

### 3.5 Env (`uses env`)

```
enum EnvErr { Missing(name: Str) }

fn (e: Env) get(name: Str) -> Str?
fn (e: Env) vars() -> Map[Str, Str]
fn (e: Env) args() -> List[Str]              // without the program name
fn (e: Env) program() -> Str
fn (e: Env) home() -> Str ! EnvErr
fn (e: Env) only(names: List[Str]) -> Env   // attenuation (below)
fn (e: Env) config_dir() -> Str ! EnvErr     // where configuration goes (below)
fn (e: Env) cache_dir() -> Str ! EnvErr
```

`program()` is the name the program was started with (argv[0]), and
`home()` the home directory (`HOME`; on Windows `USERPROFILE`, read when
the `Env` admits `HOME`); on Windows both use `/` separators (3.1).

`config_dir()` and `cache_dir()` (decision 0163) are read through the
`Env`, so attenuation and fakes apply: `XDG_CONFIG_HOME` (or
`XDG_CACHE_HOME`) when it holds an absolute path (`/x`, or `C:/x`), on
every OS; else
`<home>/.config` (`<home>/.cache`) on Linux and other Unix systems,
`<home>/Library/Application Support` (`<home>/Library/Caches`) on macOS,
`%AppData%` (`%LocalAppData%`) on Windows, with `/` separators. A
missing home is `Err(Missing("HOME"))` (as `home()`), a missing Windows
variable `Err(Missing("AppData"))`. The program adds its own directory:
`os.fs.sub(os.env.config_dir()? + "/mytool")`. Every backend (the
native ones read `HOME` for the home directory).

`e.only(names)` (decision 0156) is an `Env` that reads only the listed
variables: `get` of another name is `None`, `vars` holds only listed
names, and `home` is `Err(Missing("HOME"))` unless `HOME` is listed;
`args` and `program` are unchanged. Attenuating again keeps both lists.
Native builds have it too (decision 0172).

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

// terminals and binary input (decision 0162)
fn (s: Stdio) stdin_is_tty() -> Bool         // also stdout_is_tty, stderr_is_tty
fn (s: Stdio) width() -> Int?                // columns of the terminal; None without one
fn (s: Stdio) read_secret(prompt: Str) -> Str? ! IoErr   // a line without echo (passwords)
fn (s: Stdio) read_bytes(max: Int) -> Bytes? ! IoErr     // any bytes; None at end of input
fn (s: Stdio) read_all_bytes() -> Bytes ! IoErr
```

The reads of standard input are cancellation points while they wait: a
cancelled task, and every reader once a stop is requested (core 7.5), gets
`Err(IoErr.Canceled)` at once. Input read after a reader gave up is kept
for the next read (decision 0088). The native backends read standard
input the same way (decision 0102); a test binary's standard input is
empty there.

Terminals (decision 0162), on every backend:

- `stdin_is_tty`, `stdout_is_tty`, `stderr_is_tty` tell whether the
  stream is a terminal (a terminal's settings can be read: termios on
  Unix, a console mode on Windows); a pipe, a file, or `/dev/null` is not.
- `width()` is the columns of the terminal on standard output, else
  standard error, else standard input; `None` when none is a terminal.
  `COLUMNS` is not read (it is a variable: read it through `Env`).
- `read_secret(prompt)` writes `prompt` to standard error, turns the
  terminal's echo off while it reads a line of standard input (as
  `read_line`), restores it (also when the read is cancelled), and then
  writes the newline the user typed, which was not echoed. When standard
  input is not a terminal it reads a line without touching anything.
- `read_bytes(max)` returns at most `max` bytes (at most 1 MiB) of
  standard input as they arrive, any bytes, `None` at the end;
  `read_all_bytes()` the rest of the input. They share the input buffer
  of `read_line` and `read_all`, so text and binary reads can be mixed.
- Targets without terminal support (wasip1, js, plan9, Linux on mips and
  ppc) report no terminal and read secrets with echo on.

### 3.7 Proc (`uses proc`)

```
struct ProcOutput { code: Int, stdout: Bytes, stderr: Bytes }
enum ProcErr { NotFound(cmd: Str), Io(msg: Str), Canceled, Denied(cmd: Str) }

fn (p: Proc) exit(code: Int) -> Never        // exits immediately; defer blocks do not run
fn (p: Proc) set_grace(d: Duration)          // time between stop requested and cancel (default 30s)
fn (p: Proc) run(cmd: Str, args: List[Str]) -> ProcOutput ! ProcErr
fn (p: Proc) only(cmds: List[Str]) -> Proc   // attenuation (below)
fn (p: Proc) cmd(name: Str, args: List[Str] = []) -> Cmd       // a command, not started (3.8)
fn (p: Proc) pipeline(cmds: List[Cmd]) -> ProcOutput ! ProcErr // (3.8)
fn (p: Proc) kill(pid: Int, signal: Str = "TERM") -> () ! ProcErr  // signals another process
```

`kill` (decision 0190) sends the named signal to the process `pid`: one
of `HUP`, `INT`, `QUIT`, `KILL`, `TERM`, `USR1`, `USR2`, `CONT`, `STOP`
(the names of `kill -l` without `SIG`; another name is
`Io("unknown signal \"NAME\"")`). A `pid` of 0 or less (a process group
for kill(2)) is `Io("invalid process id N")` and nothing is sent. A
failure is `Io` with the system's text (`no such process`, `operation
not permitted`). An attenuated `Proc` sends signals only when every list
names `"kill"` (`os.proc.only(["kill"])`), else `Denied("kill")`. Record
and replay record it (`proc.kill`). Where the system has no signals
(Windows), only `KILL` is sent: it ends the process with the status
137 (128 plus SIGKILL's number, as on Unix), and another name is
`Io("signal NAME is not supported on this system")`. Used by `xo dump` (core 11.4) to ask a
running program for its task tree.

`p.only(cmds)` (decision 0156) is a `Proc` whose `run` fails with
`Denied(cmd)` for a command not listed (compared as written: `git` and
`/usr/bin/git` are different entries); `exit` and `set_grace` are
unchanged. A `Cmd` made by an attenuated `Proc` keeps its lists (3.8).
Native builds have it too (decision 0172).

The native backends run commands with `posix_spawn` (on Windows
`CreateProcessW`, decision 0197), reading the output on the event loop;
cancelling the task kills the command (decision 0107). On Windows a
command name without `\`, `/`, or `:` is looked up in `PATH` (`;`
separated) with the extensions of `PATHEXT`, as Go's exec.LookPath; the
arguments are quoted for the command line as Go's syscall.EscapeArg
does, and also when they hold a line end or another space character
(the MSYS2 and Cygwin runtimes split there).

Returning `Err` from `main` prints the error with its frames to standard error
and exits with status 1. Prefer that over `exit` for failures.

### 3.8 Commands, children, and pipelines

Status: implemented on the Go backend and its fakes (decision 0161,
2026-10-11) and natively (decision 0172, 2026-10-11: `posix_spawn` in
the C runtime, pipes on the event loop; FakeProc in native test
binaries).

```
struct Cmd {}                                 // a command that has not run; a value
fn (c: Cmd) dir(path: Str) -> Cmd             // working directory (relative: to the program's)
fn (c: Cmd) env(e: Env) -> Cmd uses env       // the child's environment is exactly e.vars()
fn (c: Cmd) set_env(name: Str, value: Str) -> Cmd
fn (c: Cmd) stdin(data: Bytes) -> Cmd         // its standard input
fn (c: Cmd) inherit() -> Cmd                  // standard streams are the program's (the terminal)
fn (c: Cmd) merge_stderr() -> Cmd             // standard error into the standard output stream
fn (c: Cmd) output() -> ProcOutput ! ProcErr uses proc   // run to the end, collect the streams
fn (c: Cmd) run() -> Int ! ProcErr uses proc            // run on the terminal; the exit status
fn (c: Cmd) spawn() -> Child ! ProcErr uses proc        // start; returns at once

struct Child {}                               // a started command; a handle
fn (c: Child) pid() -> Int
fn (c: Child) write(data: Bytes) -> () ! ProcErr uses proc        // to its standard input
fn (c: Child) close_stdin()
fn (c: Child) read(max: Int) -> Bytes? ! ProcErr uses proc        // its output; None at the end
fn (c: Child) read_err(max: Int) -> Bytes? ! ProcErr uses proc    // its error stream
fn (c: Child) read_line() -> Str? ! ProcErr uses proc             // a line of its output
fn (c: Child) wait() -> Int ! ProcErr uses proc                   // closes its input; the status
fn (c: Child) kill()
```

```
fn changed_files(p: Proc, repo: Str) -> List[Str] ! ProcErr uses proc {
  let out = p.cmd("git", ["status", "--porcelain"]).dir(repo).output()?
  (out.stdout.to_str() ?? "").lines().filter(fn(l) { !l.is_empty() })
}

fn edit(p: Proc, env: Env, path: Str) -> Int ! ProcErr uses (proc, env) {
  let editor = env.get("EDITOR") ?? "vi"
  p.cmd(editor, [path]).run()?   // the editor gets the terminal
}

fn follow(p: Proc, out: Stdio) -> () ! ProcErr uses (proc, stdio) {
  let child = p.cmd("tail", ["-f", "app.log"]).spawn()?
  while let Some(line) = child.read_line()? {
    out.println("log: ${line}")
  }
}
```

- `cmd` builds; nothing runs until `output`, `run`, `spawn`, or
  `pipeline`. The builder methods return a changed copy, so a `Cmd`
  can be prepared once and run several times.
- Environment: without `env`, the child inherits the program's
  environment (as `run`). `env(e)` replaces it with exactly the
  variables `e.vars()` shows when `env` is called, so an attenuated
  `Env` narrows it: `p.cmd("make").env(os.env.only(["PATH", "HOME"]))`.
  `set_env` adds or replaces one variable on top of either.
- Standard input: the `stdin` data, else the program's own with
  `inherit`, else empty for `output` and `run`'s terminal, and a pipe
  written with `Child.write` for `spawn` (closed by `close_stdin` or
  `wait`). Standard output and error: collected by `output`, the
  program's own with `run` or `inherit`, pipes read with `Child.read`
  and `read_err` for `spawn`. `merge_stderr` sends the error stream
  into the output stream (`output`'s `stderr` is then empty).
- Exit status: the child's exit code; a child ended by a signal is 128
  plus the signal number. On Windows `Child.kill` (and cancellation)
  ends the child with the status 137, as SIGKILL does on Unix.
- `read` returns at most `max` bytes (at most 1 MiB) as they arrive and
  `None` at the end of the stream; `read_line` strips `\n` and `\r\n`.
  Reads and `wait` are cancellation points; cancelling the task that
  spawned a child kills it. Reading a stream that is not a pipe
  (inherited or merged) is `Io`. A child that fills one pipe while the
  program waits on the other stops: read both from separate tasks, or
  merge them.
- `pipeline(cmds)` connects each command's standard output to the next
  one's input, runs them together, and returns the last one's output,
  the error streams of all of them in command order, and the status of
  the rightmost command that failed (bash's `pipefail`), else 0. The
  first command's `stdin` data feeds the pipeline; `merge_stderr` on a
  command sends its errors down the pipe. An empty list is an empty
  output.
- Attenuation (decision 0156): every command is checked against the
  lists of the `Proc` that made it before it starts (`Denied(name)`);
  for a pipeline, every command before any starts.
- Tests: `FakeProc.cmd` builds commands answered by `on_run(name, out)`,
  as `run` is (the standard input is ignored): `output` returns it,
  `run` writes its streams to the test's `FakeStdio` and returns its
  code, `spawn` gives a `Child` reading it (`pid` is 0, `write` and
  `kill` do nothing), and `pipeline` gives the last command's output
  with the error streams and failing statuses of all. A command with no
  `on_run` is `NotFound`.
- Record and replay (core 11.3) record `output`, `run`, and `pipeline`;
  a `Child`'s reads are not recorded yet.
- Native builds (decision 0172) differ in what cancellation does to a
  running command: the command is killed when the cancelled task waits
  on it (`output`, `run`, `pipeline`, `Child.wait`, `Child.read`), and
  that call is `Canceled` (the Go backend kills it at once and its
  `wait` gives the status 137). An attenuated `Proc` or `Env` debugs as
  `<proc>` or `<env>`, without its lists.

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

A fault in a handler or a middleware is isolated: 500, logged, crash
bundle written (the middleware registered before a faulting one sees no
response: the 500 replaces the whole chain's). On the
native backends (`--no-gc`, decisions 0074, 0083, and 0085) it is isolated
the same way (the handler unwinds, the server keeps serving) but no crash
bundle is written, and the error of a 500 is logged without frames.
Streamed bodies (`http.stream`) are sent natively as net/http sends them.

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
fn (s: Server) with_tls(certs: List[TlsCert]) -> Server   // HTTPS (4.6)
enum HttpErr { Io(msg: Str), Canceled }
```

`serve` runs until its scope is cancelled, then stops accepting, waits for
in flight requests, and returns `Ok(())`. Each request runs as an isolated
task with its own span and request id. Its memory may come from a request
scoped arena (core 5.5, decision 0066), which no program can observe. A
response with a body (not 1xx, 204, or 304) that is not a stream is sent
with a `Content-Length` header unless the handler set one; only streamed
bodies use chunked transfer encoding. To run a server alongside background
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
fn (c: Client) with_roots(pem: Bytes) -> Client ! TlsErr   // 4.6

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
`http` or `https` scheme and a host; anything else is a `UrlErr`. An
`https` URL is requested over TLS (4.6).

### 4.6 TLS (HTTPS)

```
struct TlsCert {}                                  // handle: a certificate chain with its private key
pub fn tls_cert(cert_pem: Bytes, key_pem: Bytes) -> TlsCert ! TlsErr
struct TlsErr { msg: Str }
fn (s: Server) with_tls(certs: List[TlsCert]) -> Server
fn (c: Client) with_roots(pem: Bytes) -> Client ! TlsErr
```

```
fn serve_https(os: Os, r: http.Router) -> () ! Error uses (fs.read, net.listen) {
  let cert = http.tls_cert(os.fs.read("cert.pem")?, os.fs.read("key.pem")?)?
  http.server(os.net.listen(":8443")?, r).with_tls([cert]).serve()?
}
```

- `tls_cert` takes a PEM certificate chain (leaf first) and the PEM private
  key of its leaf (PKCS #1, PKCS #8, or SEC 1; RSA or ECDSA), and checks
  that they match. Its errors are Go's `tls.X509KeyPair` texts, for example
  `tls: private key does not match public key`. Reading the files is the
  caller's (with `Fs`), so no capability is hidden in the call.
- `with_tls` returns the server speaking HTTPS on its listener: TLS 1.2
  and 1.3 with Go's default cipher suites and ECDHE, `certs[0]` unless the
  client's server name (SNI) is one of another certificate's DNS names
  (wildcards in the leftmost label). An empty list makes `serve` fail with
  `Io("http: with_tls needs a certificate")`. Everything else in 4.4 holds
  (shutdown, the arena, `Content-Length`); a client that sends plain HTTP
  gets `400` with the body `Client sent an HTTP request to an HTTPS
  server.` and the connection closes. Failed handshakes are not logged.
  Client certificates are not requested.
- HTTP/2 (decision 0121): over TLS a server offers `h2` and `http/1.1`
  by ALPN and prefers `h2`, and the client offers both; plain `http` is
  always HTTP/1.1 (no h2c). Nothing in the API changes, and every backend
  behaves as net/http's HTTP/2 does: a request from Go's client (or Xo's)
  carries `User-Agent: Go-http-client/2.0` and no `Connection` field,
  several `Cookie` fields arrive joined with `; `, and a response's
  `Connection` field is not sent (`close` ends the connection after its
  streams). Each request runs as a task of its own, many at once on a
  connection. Limits per connection: 250 concurrent streams, a header
  list of 1048896 bytes counting 32 per field (a larger one is answered
  `431`), 1 MiB flow control windows and frames; a client that resets
  streams faster than their handlers end is disconnected. A fault in a
  streamed body resets its stream, so the client gets
  `Protocol("stream error: stream ID 3; INTERNAL_ERROR; received from
  peer")` (a failure while a response body is read has no method and
  URL in its text, as on Go). When `serve` stops, streams in flight
  finish. The client keeps an HTTP/2 connection for later requests to
  the same host, as Go's Transport does (stream ids go up on it).
- The client verifies an `https` server's certificate chain and its host
  name (an IP address against the certificate's IP addresses) against the
  system's roots: macOS's trust settings, the CA bundle and directories of
  Linux (`SSL_CERT_FILE` and `SSL_CERT_DIR` override them there).
  `with_roots` returns a client that trusts the certificates of `pem`
  instead (for a private CA or a test certificate); `TlsErr("tls: no
  certificates found in PEM input")` when it has none. The TLS handshake
  must finish within 10 seconds and within the client's `timeout`, else
  `Timeout`.
- TLS failures are `ClientErr`s with Go's texts: a certificate that does
  not verify is `Protocol("Get \"https://...\": tls: failed to verify
  certificate: x509: ...")` (`certificate signed by unknown authority`,
  `certificate is valid for a, b, not c`, `certificate has expired or is
  not yet valid: ...`), a server answering plain HTTP is `Protocol("...:
  http: server gave HTTP response to HTTPS client")`, and an alert of the
  server (no shared cipher suite, a required client certificate) is
  `Connect("remote error: tls: handshake failure")`.
- Tests: `FakeNet` (8.1) and in memory listeners serve `https` URLs and
  `with_tls` servers in process, without TLS.
- The native backends (`--no-gc`) implement this in Xo (decision 0202):
  TLS 1.3 and 1.2, client and server, Go 1.26's defaults and texts.
  The client offers Go 1.26's groups in its order (the post quantum
  hybrids `X25519MLKEM768`, `SecP256r1MLKEM768` and
  `SecP384r1MLKEM1024`, then X25519, P-256, P-384 and P-521) and takes
  ECDSA certificates on P-256, P-384 and P-521 (P-224 too in chains and
  on TLS 1.2, as Go); the server takes `X25519MLKEM768`, X25519, P-256
  and P-384, and P-256 and P-384 ECDSA certificate keys. On macOS the
  system's roots are checked by the Security framework through the same
  calls Go makes. `XO_LLRT_C=tlsio` and
  `XO_LLRT_C=tlssrv` select the earlier Mbed TLS client and server
  (decision 0105): the same texts, except that Ed25519 certificate keys
  are not accepted and the key exchange never uses the hybrids or P-521.
- Environment switches, for compatibility and debugging (read by the
  runtime, every backend unless noted):
  - `XO_TLS_MLKEM=0`: the client offers none of the ML-KEM hybrids, so
    X25519 is its first and only key share (groups X25519, P-256, P-384,
    P-521), as Go's `GODEBUG=tlsmlkem=0`; for a server that mishandles a large
    ClientHello, and for handshake measurements without ML-KEM.
  - `XO_HTTP_MAX_BODY=<bytes>` (native backends): the largest request
    body the server reads (default 64 MiB; a longer one is cut short as
    a body whose client went away). Go has no such limit.

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
| tuple `(A, B, ...)` | array `[a, b, ...]`; decodes only from an array of exactly that length (decision 0088) |
| `()` | `null` |
| `Duration` | string `"1.5s"` (any `Duration.parse` form decodes) |
| `Time` | RFC 3339 string |
| `Bytes` | base64 string |

Builtin types listed above implement `Json`, so `http.json(200, {"msg": "hi"})`
works without a struct.

---

## 6. std/log

The log functions perform the `log` effect (core 6.1, decision 0156): no
capability (there is nothing to pass from `main`), but a `pub` function
that logs declares `uses log`. `configure` performs `stdio`, not `log`.

```
pub fn debug(msg: Str, fields: Map[Str, Debug] = {}) uses log   // also info, warn, error
pub fn configure(out: Stdio, level: Level = Level.Info, format: Format = Format.Text) uses stdio
enum Level { Debug, Info, Warn, Error }
enum Format { Text, Json }
struct Record { level: Level, msg: Str, fields: Map[Str, Str], module: Str }   // as captured in tests (8.6)
```

```
log.info("listening", {"addr": addr, "port": port})
```

Every record includes the time. A record written by the code of a
dependency (a required module version, not the main module or a
workspace module) names it, `<module path>@<version>` (decision 0173):
text records put it in brackets after the level
(`2026-10-11T09:00:00Z WARN [github.com/acme/billing@v1.2.0] retrying`),
JSON records as the key `"module"`, which replaces a field of the same
name (a dependency cannot attribute its record to another module), and
`Record.module` holds it in tests (`""` for the main module's records).
The call's source file decides: a closure of the main module run by a
dependency logs as the main module. Records of the main module carry no
attribution, on every backend (cmd/xo TestLogAttribution checks the
text and JSON records and `Record.module` on Go, arm64, and LLVM).
`xo audit` lists each dependency's log
call sites with the attribution its records carry.

**[later]** The current span and the
request id (core 11.6) are not attached yet. Without `configure`, logs go
to standard error as text at `Info`.

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

**Resource budgets** (decision 0173). `task.with_budget` bounds the work
of a call, typically a dependency's, against compute and memory denial of
service: `cpu` is running time (time its tasks spend outside blocking
waits, measured on the real monotonic clock, also under a `FakeClock`),
`alloc` is bytes allocated, and `steps` counts loop iterations and calls,
a deterministic unit. The budget covers every task started inside the
call, wherever it is started, and nested budgets all apply. Code polls
the budget at the start of every loop iteration and at the entry of
every function that calls user code, so under a budget a pure loop or a
recursion is a cancellation point. The polls are compiled only into a
program that calls `with_budget` somewhere (one without a budget is
unchanged); there they cost a nil check outside a budget. Ceilings are checked every 1024 steps of a thread (the last
quantum before a `steps` ceiling is shortened to hit it exactly).
Deferred code runs shielded and is not unwound; budgeted shielded code
that goes past twice a ceiling faults (`XO-F008`). Limits: on the Go
backend `alloc` is the process wide allocation since the budget started
(Go keeps no count per goroutine), an upper bound that also counts other
tasks running at the same time; one std operation (a large `repeat`, a
sort) is not interrupted; functions compiled to assembly by `--hybrid`
do not poll. The native backends (`--no-gc`, decision 0175) poll at
every loop iteration and at the entry of every function of the
program's own modules (leaf functions too, so a step count runs a little
higher than on the Go backend) and count `alloc` exactly per task (the
bytes each task allocated); `XO-F008` there names the `with_budget` call
as its location.

```
enum Timed[E] { TimedOut, Failed(from E) } derive Display
enum Isolated[E] { Faulted(info: Fault), Failed(from E) } derive Display
struct Fault { msg: Str, location: Str, frames: List[Str] } derive Display

pub fn with_timeout[T, E, fx: Effects](clock: Clock, d: Duration, f: fn() -> T ! E uses fx) -> T ! Timed[E]
    uses (clock, fx)
    // runs f in a child scope; on expiry cancels it, waits for it to unwind, returns Err(TimedOut)

pub fn isolate[T, E, fx: Effects](f: fn() -> T ! E uses fx) -> T ! Isolated[E] uses fx

enum Budgeted[E] { Exceeded(resource: Str), Failed(from E) } derive Display
struct Budget { cpu: Duration, alloc: Int, steps: Int }    // 0: no ceiling
pub fn budget(cpu: Duration = 0s, alloc: Int = 0, steps: Int = 0) -> Budget
pub fn with_budget[T, E, fx: Effects](clock: Clock, b: Budget, f: fn() -> T ! E uses fx) -> T ! Budgeted[E]
    uses (clock, fx)
    // runs f in a child scope under the budget; when a ceiling is passed cancels
    // the scope, waits for it to unwind, returns Err(Exceeded("cpu" | "alloc" | "steps"))

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

Native test binaries (`xo test --no-gc`, decision 0083) give the same fakes:
the same virtual clock (from 2026-01-01T00:00:00Z), the same seeded `Rand`
values, in memory listeners on ports from 49152, FakeNet handlers and
failures, captured streams and log records, MemFs (decision 0108),
`set_input` and reading standard input, FakeProc's `run` and `on_run`
(decision 0107), and `os.tasks` (canceled when the test ends, decision
0109). `os.proc.exit` ends the test (it passes), and inside `run_main`
unwinds main back to it. `testing.gen` and property tests make the Go
backend's values for a seed, and a failing property test reports the
same case and shrunk input (decision 0120). With `xo test --real` a test
gets real capabilities for the effects it declares (with `clock`, `net`,
or `proc` its clock is the real one).

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
`only` on `FakeNet`, `FakeEnv`, and `FakeProc` attenuates the fake as
on the real capability (3.2, 3.5, 3.7): a test can check that a
library given `os.net.only([...])` fails with `Denied` elsewhere.

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
// FakeClock also has sleep_or_cancel (3.3), on virtual time
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

MemFs implements the handles, links, globs, and locks of 3.1: a read
handle reads the contents as they were at `open`; a write handle writes
through at once; permission bits default to `0o644` for a file and
`0o755` for a directory; `read`, `read_text`, `open`, `copy`, `stat`, and
`exists` follow symbolic links (a chain of more than 40 is `Io`); a lock
held by one task makes another task's `lock` wait (deadlocks are
reported as for a `Mutex`). `fail_next` applies with `Read` (open, copy)
and `Write` (create, open_append, append, set_mode, symlink, lock).

### 8.4 FakeEnv, FakeStdio, FakeProc

```
fn (e: FakeEnv) set(name: Str, value: Str)
fn (e: FakeEnv) set_args(args: List[Str])
fn (s: FakeStdio) set_input(text: Str)
fn (s: FakeStdio) set_input_bytes(data: Bytes)        // set_input for any bytes
fn (s: FakeStdio) set_tty(width: Int)                 // the streams are terminals of width columns (0: not)
fn (s: FakeStdio) output() -> Str                     // everything printed to stdout
fn (s: FakeStdio) errors() -> Str                     // everything printed to stderr
fn (p: FakeProc) exit_code() -> Int?                  // set when exit was called
fn (p: FakeProc) signal_after(d: Duration)            // next run_main gets SIGTERM after d (virtual time)
fn (p: FakeProc) on_run(cmd: Str, out: ProcOutput)    // also answers Cmd runs (3.8)
fn (p: FakeProc) killed() -> List[Str]                // the signals kill recorded, "NAME PID" (3.7)
```

`FakeProc.kill(pid, signal)` checks its arguments as `Proc.kill` does and
records the signal instead of sending it (decision 0190).

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
registered with `rt.RegisterSqlDriver`; none ships yet. Native builds
(`--no-gc`, decision 0083) run the same in memory driver, ported to Xo,
with the same results and error texts; there `sql.open` always reports an
unknown driver (no Go drivers), `max_open` does not bound the in memory
driver (it runs one statement at a time), and a `Row`'s Debug text is its
fields.

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
into the runtime (no module dependency). Native builds (`--no-gc`,
decision 0083) compute the same results in C (SHA-256, HMAC, PBKDF2,
BLAKE2b, Argon2, SHA-512 and Ed25519), with random bytes from the operating system
(`getentropy` on macOS, `getrandom` on Linux).

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
pub fn ed25519_public_key(seed: Bytes) -> Bytes                   // 32 bytes; faults unless seed is 32 bytes
pub fn ed25519_sign(seed: Bytes, msg: Bytes) -> Bytes             // 64 bytes; faults unless seed is 32 bytes
pub fn ed25519_verify(public_key: Bytes, msg: Bytes, sig: Bytes) -> Bool
```

- Hashing, signing, and comparison are pure: no capability, no effect.
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
- Ed25519 (RFC 8032, decision 0189; added 2026-10-12): the private key is
  a 32 byte seed; make one with `random_bytes(r, 32)`, keep it secret, and
  publish `ed25519_public_key(seed)`. `ed25519_sign` is deterministic (the
  same seed and message give the same 64 byte signature, so signing needs
  no `Rand`). `ed25519_verify` is `false`, never a fault, for a key that is
  not 32 bytes, a signature that is not 64, an encoding that is no point
  of the curve, or a signature scalar S that is not below the group order;
  it accepts exactly what Go's `crypto/ed25519.Verify` accepts (the
  cofactorless equation compared byte for byte, the non canonical point
  encodings Go accepts). Native builds compute it in C (SHA-512 and the
  curve arithmetic in xo_crypto.c), checked against Go on random inputs
  and the RFC 8032 vectors. The checksum log of the module commands
  (decision 0124) signs and verifies tree heads with it.

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

---

## 12. std/go

Values that cross the Go interop boundary (core 4.4, decision 0041).
Bindings written by `xo bind` import this module; user code needs
`use std/go` only to name the type, for example in `! go.Error`.

```
struct Error {}                      // handle; implements Display, so it converts to Error at `?`
fn (e: Error) display() -> Str       // the Go error's Error() text, captured at the boundary
fn (e: Error) message() -> Str       // the same text
fn (e: Error) go_type() -> Str       // the Go dynamic type, for example "*url.Error"
```

Calling these performs no Go call (no `go` effect). The native backends do
not build `use go` code at all (`XO1201`, impossible).

---

## 13. std/time

Status: **Accepted** (decision 0094, 2026-10-09; drafted by the spec
audit, `bench/results/spec-audit-2026-10-09.md`). Implemented on the Go
backend (2026-10-09): `std/time.xo`, `std/rt/timezone.go`,
`timecivil.go`, `timefmt.go`; the compile time layout check is
selfhost/check's (`internal/check/timelayout.go` until decision 0215). The native backends build all of it
(2026-10-09: `std/native/time.xo` over `std/llrt/xo_time.c`, the same
zone data per use, output identical to the Go backend's; decision 0094
"Native implementation"; decision 0089 makes native a preview at 1.0;
since 2026-10-13 the zone rules, TZif parsing, the POSIX TZ footer,
and the lookup, are Xo in `std/native/llrt/tz.xo`, decision 0193 step
5, and xo_time.c keeps the zone data and the table of loaded zones).
Points
the API text below leaves open are settled in 13.5. Instants,
durations, Unix seconds and milliseconds, and RFC 3339 in UTC stay in
the prelude (2.6).

Principles:

- `Time` stays an instant with no zone. A zone is a separate value, and a
  civil (wall clock) reading is a separate type, so code never compares a
  zoned and an unzoned value by accident.
- Zone data comes with the program, embedded per use, so loading a zone
  by name is pure, needs no capability, and gives the same answer on every
  OS and in tests. `utc()` and `fixed` need no data; a zone name that is a
  constant embeds only that zone (about 1 to 3 KB); a name known only at
  run time embeds the whole IANA database (about 400 KB compressed) and
  `xo build` prints a note naming the call that caused it.
  `--tzdata=system` reads the host's database instead (smaller binaries,
  host dependent answers). Only the machine's own zone needs authority:
  `local` takes `Env` (`TZ`, then the system setting), so a function that
  depends on where it runs says so.
- Layouts are written with named, lowercase fields, not Go's reference
  date, not C's `%` codes, and not the letter patterns of Java, C#, and
  Swift, where `MM` (month) and `mm` (minute) differ only in case:
  `"{year}-{month}-{day} {hour}:{minute}:{second}"`. Common formats are
  constants.
- Parsing returns a `Result` whose error names the position and the field
  expected, like `json.DecodeErr`.
- Daylight saving gaps and overlaps are explicit: converting a civil time
  that does not exist or exists twice is an error unless the caller picks
  a rule.

### 13.1 Zones

```
struct Zone {}                                   // handle; Eq, Display (its name)
enum ZoneErr { Unknown(name: Str), NoLocal(msg: Str) }
fn (e: ZoneErr) display() -> Str                 // unknown time zone "Mars/Olympus"

pub fn utc() -> Zone
pub fn zone(name: Str) -> Zone ! ZoneErr         // IANA name: "Europe/Berlin"; pure (embedded data)
pub fn fixed(name: Str, offset: Duration) -> Zone   // "UTC+2" style; faults past 24h
pub fn local(env: Env) -> Zone ! ZoneErr uses env   // TZ, then the system zone

fn (z: Zone) name() -> Str
fn (z: Zone) offset_at(t: Time) -> Duration      // offset from UTC at that instant
fn (z: Zone) abbreviation_at(t: Time) -> Str     // "CET", "CEST"
```

In tests `os.env` is a fake, so `local(os.env)` is `utc()` unless the
test sets `TZ`.

### 13.2 Civil time

```
struct Date { year: Int, month: Int, day: Int } derive Ord, Json
struct ClockTime { hour: Int, minute: Int, second: Int, nanos: Int } derive Ord, Json
struct Civil { date: Date, time: ClockTime, offset: Duration, zone: Str } derive Json
enum Weekday { Monday, Tuesday, Wednesday, Thursday, Friday, Saturday, Sunday } derive Display, Ord, Json
enum Ambiguity { Earlier, Later, Reject }
enum CivilErr { Invalid(msg: Str), Skipped(at: Str), Repeated(at: Str) }
fn (e: CivilErr) display() -> Str                // invalid civil time: month 13 out of range 1 to 12

pub fn civil(t: Time, z: Zone) -> Civil
pub fn instant(date: Date, time: ClockTime, z: Zone, ambiguity: Ambiguity = Ambiguity.Reject) -> Time ! CivilErr
pub fn date(year: Int, month: Int, day: Int) -> Date ! CivilErr
fn (d: Date) weekday() -> Weekday
fn (d: Date) add_days(n: Int) -> Date
fn (d: Date) add_months(n: Int) -> Date          // clamps the day: Jan 31 + 1 month is Feb 28 or 29
fn (d: Date) days_until(o: Date) -> Int
fn (d: Date) display() -> Str                    // "2026-10-09"
```

`instant` with `Reject` returns `Skipped` for a time inside a spring
forward gap and `Repeated` for one inside a fall back overlap; `Earlier`
and `Later` pick the first or second instant (for a gap, the instant the
offset before or after the change gives). Arithmetic on `Time` stays exact
(`t + 24h` is always 24 hours); calendar arithmetic is on `Date`.

### 13.3 Formatting and parsing

```
const RFC3339: Str = "{year}-{month}-{day}T{hour}:{minute}:{second}{frac}{offset}"
const RFC1123: Str = "{weekday:short}, {day} {month:short} {year} {hour}:{minute}:{second} {zone}"   // HTTP dates
const DATE: Str = "{year}-{month}-{day}"
const TIME: Str = "{hour}:{minute}:{second}"

struct ParseErr { input: Str, layout: Str, at: Int, msg: Str }
fn (e: ParseErr) display() -> Str                // at byte 5 of "2026-1x-09": expected 2 digit month
struct LayoutErr { layout: Str, msg: Str }
fn (e: LayoutErr) display() -> Str               // bad layout "{yr}": unknown field {yr}

pub fn format(t: Time, z: Zone, layout: Str) -> Str             // faults on a bad layout (constant layouts are checked at compile time)
pub fn parse(s: Str, layout: Str, z: Zone) -> Time ! ParseErr   // z applies when the text has no offset
pub fn check_layout(layout: Str) -> () ! LayoutErr
```

Fields, one spelling each: `{year}`; `{month}` 01 to 12, `{month:1}`
without padding, `{month:short}` `Jan`, `{month:name}` `January`; `{day}`
01 to 31, `{day:1}`; `{weekday:short}` `Mon`, `{weekday:name}` `Monday`;
`{hour}` 00 to 23, `{hour12}` 01 to 12 with `{ampm}` AM or PM; `{minute}`;
`{second}`; `{frac}` a fraction with as many digits as needed (none when
zero), `{frac:3}` exactly three (1 to 9); `{offset}` `Z` or `+02:00`,
`{offset:compact}` `+0200`; `{zone}` the zone abbreviation. A literal
brace is written `{lbrace}` or `{rbrace}`. A layout that is a constant is
checked at compile time: an unknown field is an error with a did you mean
fix. Letter patterns and `%` codes from other languages (`{yyyy}`,
`{MM}`, `%Y`) are the habit error `XO0177` with a machine fix to the named
form (decision 0087).
Names are English only; locales are out of scope. Parsing accepts exactly
the layout (no guessing between formats); a two digit year is not offered.

### 13.4 Decided points

- Zone data is embedded per use (above), with `--tzdata=system` as the
  opt in alternative.
- `format` with a bad layout built at run time faults, like a bad index;
  `check_layout` returns a `Result` for code that builds layouts from
  input.
- Native backends carry the same embedded data in C; the Go backend uses
  Go's `time` package with the per use data. Implementation order: Go
  backend, checker rules, native runtime, cross backend diff tests.

### 13.5 Implementation notes (2026-10-09)

Settled while implementing; examples are in the goldens
`internal/gen/testdata/golden/time_*` and
`internal/check/testdata/golden/time_layout`.

- **Zone data.** The zone data is the `lib/time/zoneinfo.zip` of the Go
  toolchain that builds the program (`XO_ZONEINFO` names another file).
  `time.zone("UTC")` needs no data. A constant name is a string literal,
  a `const`, or a `+` of them; a constant name the database does not have
  gets a build note and returns `ZoneErr.Unknown` at run time. `time.zone`
  used as a function value counts as a name known only at run time.
  Names are case sensitive. Measured on darwin/arm64 (2026-10-09): a
  program using only `utc()` is 7,968,146 bytes, one constant zone
  (Europe/Berlin) 7,968,994 (+848), a run time name 8,382,498 (+414 KB,
  the database), the same with `--tzdata=system` 7,969,666.
- **`local(env)`.** `TZ` from the `Env` (a leading `:` is dropped; empty
  or `UTC` is `utc()`); a name that is not embedded is looked up in the
  host database, since `local` is host dependent by nature. Without
  `TZ`, a fake `Env` gives `utc()` and the real one the system zone,
  named by the `/etc/localtime` link when there is one (`Local`
  otherwise).
- **Civil time.** `Civil.zone` is the zone's name (`Europe/Berlin`), not
  the abbreviation (`abbreviation_at` gives that). `date` accepts years 1
  to 9999; `Time` covers about 1678 to 2262, and `instant` outside it is
  `CivilErr.Invalid`. Date methods on a `Date` built with a struct
  literal and out of range fields normalize it (month 13 is January of the
  next year). For a gap, `Earlier` uses the offset before the change and
  `Later` the one after: 02:30 on 2026-03-08 in New York is 03:30 EDT
  with `Earlier` and 01:30 EST with `Later` (Python's `fold=0` and `1`).
  Error display text, one sentence each, no trailing period:
  `ZoneErr.Unknown` is `unknown time zone "Mars/Olympus"`, `NoLocal` is
  `cannot determine the local time zone: <msg>`; `CivilErr.Invalid` is
  `invalid civil time: <msg>` (`month 13 out of range 1 to 12`),
  `Skipped` is `2026-03-08T02:30:00 America/New_York does not exist
  (skipped by a daylight saving change)`, `Repeated` is
  `2026-11-01T01:30:00 America/New_York happens twice (repeated by a
  daylight saving change)`; `ParseErr` is `at byte 5 of "2026-1x-09":
  expected 2 digit month` and `LayoutErr` is `bad layout "{yaer}":
  unknown field {yaer}`. The payload fields stay readable for code that
  matches on the error.
- **Layouts.** `{frac}` and `{frac:N}` include the leading `.` (so
  `{frac}` prints nothing for a whole second). `{offset}` prints `Z` for
  a zero offset, `{offset:compact}` prints `+0000`. `{hour12}` needs
  `{ampm}` in the same layout (a layout error otherwise). `{weekday}`
  alone is not a field. `LayoutErr` displays as
  `bad layout "{yaer}": unknown field {yaer}`. A constant layout error is
  `XO0420` (did you mean fix for a misspelled field); a layout written
  with Java, C#, or Swift letters (`{yyyy}`, `yyyy-MM-dd`), strftime
  codes (`%Y-%m-%d`), or Go's reference date (`2006-01-02`) is `XO0177`
  with a fix to the named fields when every piece has one (`%y`, a two
  digit year, has none).
- **Parsing.** Numeric fields have exactly their width (`{day:1}` and
  `{month:1}` take one or two digits). Fields missing from the layout
  default to 1970-01-01 and 00:00:00. A parsed weekday must match the
  date. Without `{offset}`, `{zone}` picks the instant whose abbreviation
  in `z` matches (`UTC` and `GMT` also mean +00:00), and a civil time in a
  gap or overlap with neither is a `ParseErr` (no silent choice).
- **HTTP dates.** `RFC1123` formatted in `utc()` ends in `UTC`, while
  HTTP requires `GMT`: write the zone literally,
  `"{weekday:short}, {day} {month:short} {year} {hour}:{minute}:{second} GMT"`
  with `utc()`. Parsing with `RFC1123` accepts `GMT`.

---

## 14. std/cli

Status: implemented on every backend (decision 0160, 2026-10-11): the Go
backend in `std/rt/cli.go`, the native backends in `std/native/cli.xo`,
with the same help text, usage errors, and faults (the golden programs
`internal/llvm/testdata/golden/cli_*` compare them).

Command line parsing for tools. A command is a value built by functions
(no struct tags, no reflection, no builder state to get out of order);
`parse` reads the arguments into a `Parsed` with typed accessors.

### 14.1 Declaring a command

```
struct Command {}      // handles
struct Flag {}
struct Arg {}

pub fn command(name: Str, help: Str, flags: List[Flag] = [], args: List[Arg] = [],
               commands: List[Command] = [], version: Str = "") -> Command

pub fn bool_flag(name: Str, help: Str, short: Str = "", env: Str = "") -> Flag
pub fn str_flag(name: Str, help: Str, short: Str = "", default: Str = "", env: Str = "",
                required: Bool = false, value: Str = "str") -> Flag
pub fn int_flag(name: Str, help: Str, short: Str = "", default: Int = 0, env: Str = "",
                required: Bool = false) -> Flag
pub fn duration_flag(name: Str, help: Str, short: Str = "", default: Duration = 0s, env: Str = "",
                     required: Bool = false) -> Flag
pub fn choice_flag(name: Str, help: Str, choices: List[Str], short: Str = "", default: Str = "",
                   env: Str = "", required: Bool = false) -> Flag
pub fn strs_flag(name: Str, help: Str, short: Str = "", env: Str = "", value: Str = "str") -> Flag

pub fn arg(name: Str, help: Str, optional: Bool = false) -> Arg
pub fn rest(name: Str, help: Str, min: Int = 0) -> Arg
```

```
fn app() -> cli.Command {
  cli.command("tool", "Manages widgets.",
    flags: [
      cli.bool_flag("verbose", "print more", short: "v"),
      cli.str_flag("config", "the configuration file", default: "tool.toml", env: "TOOL_CONFIG", value: "path"),
    ],
    commands: [
      cli.command("add", "Adds widgets.",
        flags: [cli.int_flag("count", "how many", short: "n", default: 1)],
        args: [cli.arg("name", "the widget name")]),
      cli.command("remove", "Removes widgets.", args: [cli.rest("names", "widget names", min: 1)]),
    ],
    version: "1.4.0")
}
```

- `name` of the root command is the program's name in help and usage
  text; a subcommand's `name` is the word that selects it. A command has
  positional arguments or subcommands, not both.
- Command names are letters, digits, `.`, `_`, and `-` (not first);
  flag names are lowercase letters, digits, and `-` (not first);
  `short` is one ASCII letter or digit. `--help` and `-h` are reserved,
  and `--version` when the root command has a `version`. Flag names are
  unique along each path of commands (a subcommand may not repeat a flag
  of a command above it; sibling subcommands may share names).
- Positional arguments: required ones first, then optional ones, then at
  most one `rest`, last.
- `env` names a variable read when the flag is not on the command line
  (an empty value counts as unset; a `strs_flag` variable is split on
  `,`). Variables are read through the `Env` given to `parse`, so an
  attenuated `Env` (`only`, 3.5) hides unlisted ones.
- A mistake in the declaration (a bad or duplicate name, a short name
  used twice, a choice default not among the choices, an argument out of
  order, arguments and subcommands on one command) is a fault at the
  `parse` or `help` call: it is a bug in the program, not in its input.

### 14.2 Parsing

```
pub fn parse(cmd: Command, args: List[Str], env: Env) -> Parsed ! CliErr uses env
pub fn parse_or_exit(cmd: Command, env: Env, stdio: Stdio, proc: Proc) -> Parsed
    uses (env, stdio, proc)
pub fn help(cmd: Command, path: Str = "") -> Str     // path: "remote add"; faults if unknown
pub fn completion(cmd: Command, shell: Str) -> Str ! CliErr   // 14.5

enum CliErr {
  Help(text: Str)               // -h or --help: the selected command's help
  Version(text: Str)            // --version: "name version"
  Usage(msg: Str, usage: Str)   // a mistake in the arguments
}
fn (e: CliErr) display() -> Str  // the text of Help and Version, the msg of Usage
fn (e: CliErr) exit_code() -> Int // 0 for Help and Version, 2 for Usage
```

`args` are the arguments without the program's name (`Env.args`).
`parse_or_exit(cmd, env, stdio, proc)` parses `env.args()`; on `Help` or
`Version` it prints the text to standard output and exits with status 0,
on `Usage` it prints `error: <msg>`, an empty line, and `usage` to
standard error and exits with status 2 (the convention of Unix tools
for a usage error):

```
fn main(os: Os) ! {
  let p = cli.parse_or_exit(app(), os.env, os.stdio, os.proc)
  match p.command() {
    "add" => add(p.arg("name"), p.int("count"), p.bool("verbose"))?
    "remove" => remove(p.rest("names"))?
    _ => {}
  }
}
```

Rules, in order:

- `--name value`, `--name=value`, `-n value`, `-nvalue`, `-n=value`. A
  bool flag takes no separate value: `--verbose`, `-v`, or
  `--verbose=true|false|1|0`. Short bool flags combine (`-vq`); a short
  flag that takes a value ends the cluster (`-vn5`). A value may start
  with `-` (`--name -x`). Given twice, the last value wins, except for a
  `strs_flag`, which collects every value in order.
- `--` ends the flags: every later argument is positional. `-` alone is
  positional.
- Flags may come before or after positional arguments. A subcommand is
  selected by the first positional argument of a command with
  subcommands; after it, the flags of every command on the path are
  accepted, those of the subcommand only after its name.
- `-h` or `--help` anywhere before `--` returns `Help` with the help of
  the command selected so far; `--version` (when the root has a version)
  returns `Version`.
- After the arguments: a command with subcommands and none selected is
  `missing command (one of a, b)`; then each flag of the path, in
  declaration order (root first), not given takes its variable, and a
  `required` one still missing is `missing required flag --name`; then
  positional arguments are assigned in order (`missing argument <name>`,
  also for a `rest` with fewer than `min`; `unexpected argument "x"` for
  one too many).
- Usage messages: `unknown flag --nmae (did you mean --name?)`,
  `unknown flag -x`, `unknown command "ad" (did you mean add?)`, `flag
  --count needs a value`, `invalid value "two" for --count: expected an
  integer` (`true or false`, `a duration such as 30s or 1m30s`, `one of
  text, json`), with ` (from $VAR)` before the colon for a value read
  from a variable. A suggestion is the closest name within two edits
  (fewer than the length of the word), the first declared on a tie.
- `usage` is `Usage: <path> [flags] <args>` (`<command>` for a command
  with subcommands), an empty line, and `Run '<path> --help' for more
  information.`

### 14.3 Reading the result

```
struct Parsed {}
fn (p: Parsed) command() -> Str          // the subcommand path, "" for the root ("remote add")
fn (p: Parsed) bool(name: Str) -> Bool
fn (p: Parsed) str(name: Str) -> Str     // also choice flags; the value, else the default
fn (p: Parsed) str_opt(name: Str) -> Str?
fn (p: Parsed) int(name: Str) -> Int     // also int_opt
fn (p: Parsed) duration(name: Str) -> Duration   // also duration_opt
fn (p: Parsed) strs(name: Str) -> List[Str]
fn (p: Parsed) is_set(name: Str) -> Bool // given on the command line or in its variable
fn (p: Parsed) arg(name: Str) -> Str     // a required positional argument
fn (p: Parsed) arg_opt(name: Str) -> Str?
fn (p: Parsed) rest(name: Str) -> List[Str]
```

- Flags are named without dashes. `_opt` accessors are `None` when the
  flag was not given (on the command line or in its variable); the plain
  ones return the default.
- A flag or argument of a command that was not selected reads as not
  given (its default, `None`, `[]`, `false`), so a program may read every
  flag up front; `arg` of one faults (it has no value to return).
- Reading a name that no command declares, or with the accessor of
  another type, is a fault naming the right accessor: `cli: --limit is
  declared by int_flag, so read it with int or int_opt`.

### 14.4 Help text

`help(cmd, path)` and `Help` give the same text: the command's help line
and an empty line (when it has one), the usage line, then sections
`Commands:`, `Arguments:`, `Flags:` (the command's flags, then `-h,
--help` and, when the root has a version, `--version`), and `Global
flags:` (the flags of the commands above), each row indented two spaces
and every description starting in one column, two spaces after the
widest left side. A flag's left side is `-n, --name <value>` (indented four
spaces, `--name <value>`, without a short name; no `<value>` for a bool flag), where value
is the `value` argument, `int`, `duration`, or the choices joined with
`|`. Its description is followed by `[default: x]` (when not empty or
zero), `[env: VAR]`, `[required]`, and `[repeatable]` as they apply.
No trailing newline.

```text
Manages widgets.

Usage: tool [flags] <command>

Commands:
  add                  Adds widgets.
  remove               Removes widgets.

Flags:
  -v, --verbose        print more
      --config <path>  the configuration file [default: tool.toml] [env: TOOL_CONFIG]
  -h, --help           show this help
      --version        show the version
```


### 14.5 Shell completions and versions

`completion(cmd, shell)` (decision 0165) returns a completion script for
`"bash"`, `"zsh"`, or `"fish"`, made from the declaration alone: the
program does not run while the user completes. Another shell is
`Usage("unknown shell \"x\" (one of bash, zsh, fish)", usage)`. A tool
offers it as a subcommand:

```
fn app() -> cli.Command {
  cli.command("tool", "Manages widgets.",
    commands: [
      cli.command("completion", "Prints a shell completion script.", args: [cli.arg("shell", "bash, zsh, or fish")]),
    ],
    version: VERSION)
}

fn main(os: Os) ! {
  let p = cli.parse_or_exit(app(), os.env, os.stdio, os.proc)
  if p.command() == "completion" {
    os.stdio.print(cli.completion(app(), p.arg("shell"))?)
  }
}
```

Users install it with `source <(tool completion bash)`, `tool completion
zsh > "${fpath[1]}/_tool"`, or `tool completion fish >
~/.config/fish/completions/tool.fish`. The scripts complete subcommand
names at each level, the flags accepted there (those of the commands
above included) with `-h`, `--help`, and `--version`, the values of
`choice_flag`s after their flag, and files after other flags that take a
value and where a command takes positional arguments (zsh and fish
describe each word with its help line). The text is fixed by the
implementation and the same on every backend; command names are letters,
digits, `.`, `_`, and `-`, so no quoting can break it.

A command's `version` usually comes from a constant stamped by the build:
`const VERSION: Str = "dev"` in the main module and `xo build --stamp
VERSION=1.4.0` (core 10.1, decision 0165) make `tool --version` print
`tool 1.4.0`.
---

## 15. std/term

Status: implemented on every backend (decision 0162, 2026-10-11).

Colors for terminal output that follow the user's settings. Whether a
stream is a terminal, its width, and input without echo are `Stdio`
methods (3.6).

```
struct Painter { on: Bool }       // wraps text in ANSI styles when on

pub fn color_enabled(out: Stdio, env: Env) -> Bool uses (stdio, env)
pub fn painter(out: Stdio, env: Env) -> Painter uses (stdio, env)
fn (p: Painter) bold(s: Str) -> Str   // also dim, underline, red, green, yellow, blue, magenta, cyan, gray
pub fn strip(s: Str) -> Str           // s without ANSI escape sequences
```

- `color_enabled`: `NO_COLOR` set and not empty is off (no-color.org);
  else `FORCE_COLOR` or `CLICOLOR_FORCE` set, not empty, and not `0` is
  on; else `TERM=dumb` is off; else on when standard output is a
  terminal. The variables are read through `env`, so an attenuated `Env`
  that does not list them reads as unset.
- A painter's methods return `s` unchanged when `on` is false or `s` is
  empty, else `ESC [ code m`, `s`, `ESC [ 0 m` with code 1 bold, 2 dim, 4
  underline, 31 red, 32 green, 33 yellow, 34 blue, 35 magenta, 36 cyan,
  90 gray. `Painter{on: false}` and `Painter{on: true}` are for tests.
- `strip` removes CSI sequences (`ESC [`, parameters, one final byte in
  `@` to `~`) and any other `ESC x` pair; text whose stripping would cut a
  UTF-8 sequence is returned unchanged.

```
fn report(out: Stdio, env: Env, failed: Int) uses (stdio, env) {
  let p = term.painter(out, env)
  if failed > 0 {
    out.println(p.red("${failed} failed"))
  } else {
    out.println(p.green("ok"))
  }
}
```

---

## 16. std/csv

Status: implemented on every backend (decision 0164, 2026-10-11): the
Go backend in `std/rt/csv.go`, the native backends in
`std/native/csv.xo`, the same records, text, and errors.

```
struct CsvErr { line: Int, column: Int, msg: Str }   // displays as `csv: line 3, column 5: msg`

pub fn parse(text: Str, sep: Rune = ',') -> List[List[Str]] ! CsvErr
pub fn parse_maps(text: Str, sep: Rune = ',') -> List[Map[Str, Str]] ! CsvErr
pub fn format(rows: List[List[Str]], sep: Rune = ',') -> Str
```

- RFC 4180: a field in double quotes may hold the separator, line
  breaks, and `""` for a quote; `\r\n` inside quotes reads as `\n`.
  Records end with `\n`, `\r\n`, or the end of the text; empty lines are
  skipped. Records may have different numbers of fields. Spaces are part
  of fields.
- Errors (line and column are 1-based, the column counts bytes): `" in
  an unquoted field`, `quoted field not closed` (at the opening quote),
  `unexpected character after a quoted field`.
- `parse_maps` reads the first record as the header and makes a map per
  later record, in header order; a record with another number of fields
  is `expected N fields as in the header, found M` at its first line.
- `format` writes each record and `\n`; a field is quoted when it holds
  the separator, a quote, `\r` or `\n`, starts with a space or a tab, or
  is the only, empty field of a record (else the record would read as an
  empty line). `parse(format(rows))` gives `rows` back.
- `sep` is any rune but `"`, `\r`, and `\n` (a fault otherwise); use
  `'\t'` for tab separated values.

---

## 17. std/regexp

Status: implemented on the Go backend (decision 0164, Go's `regexp`)
and natively (decision 0172: Go's regexp/syntax parser, compiler, and
Pike VM ported to Xo, the same matches and errors). Native builds have
the Unicode classes `\pL` (`\p{Letter}`), `\p{Nd}`, `\p{Any}`, and
`\p{ASCII}` so far; another class Go knows is a `RegexErr` saying it is
not supported by native builds yet.

```
struct Regex {}                          // handle
struct RegexErr { pattern: Str, msg: Str }   // `invalid regexp "(a": missing closing ): ...`

pub fn compile(pattern: Str) -> Regex ! RegexErr
pub fn must(pattern: Str) -> Regex       // faults on a bad pattern
pub fn quote(text: Str) -> Str           // matches text literally
fn (r: Regex) pattern() -> Str
fn (r: Regex) is_match(s: Str) -> Bool
fn (r: Regex) find(s: Str) -> Str?
fn (r: Regex) find_index(s: Str) -> (Int, Int)?     // byte offsets
fn (r: Regex) find_all(s: Str) -> List[Str]
fn (r: Regex) captures(s: Str) -> List[Str?]?       // [whole match, group 1, ...]
fn (r: Regex) named(s: Str) -> Map[Str, Str]?       // (?P<name>...) groups that took part
fn (r: Regex) replace_all(s: Str, repl: Str) -> Str // $1, ${name}, $$ in repl
fn (r: Regex) replace_all_literal(s: Str, repl: Str) -> Str
fn (r: Regex) split(s: Str) -> List[Str]
```

The syntax and semantics are RE2's as Go implements them: leftmost
first matching, no backreferences or lookaround, and time linear in the
input, so a pattern or input from a user cannot make matching run away.
In `replace_all`, `$name` takes the longest name it can (`$1x` is group
`1x`); write `${1}x`. Raw strings keep backslashes: `r"\d+"`.

---

## 18. std/archive

Status: implemented on the Go backend (decision 0164, Go's
`compress/gzip`, `archive/tar`, `archive/zip`) and natively (decision
0172: those packages ported to Xo, the same bytes and errors; zip64
archives are an error natively).

```
struct Entry { path: Str, data: Bytes, mode: Int, is_dir: Bool, modified: Time }
struct ArchiveErr { msg: Str }           // displays as `archive: msg`

pub fn file(path: Str, data: Bytes) -> Entry    // mode 0o644, modified at the Unix epoch
pub fn dir(path: Str) -> Entry                  // mode 0o755
pub fn gzip(data: Bytes) -> Bytes
pub fn gunzip(data: Bytes) -> Bytes ! ArchiveErr
pub fn tar(entries: List[Entry]) -> Bytes ! ArchiveErr
pub fn untar(data: Bytes) -> List[Entry] ! ArchiveErr
pub fn zip(entries: List[Entry]) -> Bytes ! ArchiveErr
pub fn unzip(data: Bytes) -> List[Entry] ! ArchiveErr
```

- In memory and pure: read the archive with `Fs.read`, write the
  entries with `Fs.write` (into a sub capability, so the archive's paths
  stay under it).
- `gzip` writes no name and no time in the header, so the output depends
  on the data only; `gunzip` reads concatenated members as one stream.
- Paths use `/`. Writing or reading an entry whose path is empty,
  absolute (`/x`, `C:/x`), or leaves the root (`../x`, `a/../../x`) is an
  error (zip slip); so is a tar or zip entry that is a link or a device.
  Directories are written with a trailing `/` and read without it. A PAX
  global header (`git archive` writes one holding the commit) describes
  the archive, not an entry, and `untar` skips it (2026-10-12).
- `tar` writes PAX headers with the entries' modes (`0o7777`) and times;
  `zip` deflates files and stores their modes and times (with Unix
  extended timestamps, so times before 1980 survive).

---

## 19. std/toml

Status: implemented on every backend (decision 0164): a TOML 1.0 parser
in `std/rt/toml.go` and the same parser in `std/native/toml.xo`, with
the same JSON text and errors (golden `toml_basic`).

```
use std/json

struct TomlErr { line: Int, msg: Str }   // displays as `toml: line 3: expected a value after =`
enum DecodeErr { Syntax(err: TomlErr), Value(err: json.DecodeErr) }   // displays as the inner error

pub fn decode[T: Json](text: Str) -> T ! DecodeErr
pub fn to_json(text: Str) -> Str ! TomlErr
```

```
struct Server { host: Str, port: Int = 8080, timeout: Duration = 30s } derive Json
struct Config { name: Str, servers: List[Server] } derive Json

fn load(fs: Fs) -> Config ! Error uses fs.read {
  toml.decode[Config](fs.read_text("app.toml")?)?
}
```

- A TOML document maps onto JSON: a table is an object (keys in
  document order), an array an array, a string a string, an integer
  (decimal, `0x`, `0o`, `0b`, underscores between digits) a JSON
  integer, a float a JSON number (its text without `+` and underscores),
  a boolean a boolean, and a date or time its RFC 3339 text (a space
  between date and time and lower case `t` and `z` become `T` and `Z`).
  `decode[T]` then follows the `derive Json` rules of section 5: missing
  keys take field defaults, unknown keys are ignored, `Duration` fields
  take `"1m30s"` text, an offset date-time decodes into `Time`, and a
  local date or time into `Str`.
- `inf` and `nan` are errors (JSON has no form for them).
- All of TOML 1.0 is read: comments, bare, quoted, and dotted keys,
  basic and literal strings and their multi-line forms (a line ending
  backslash trims the whitespace after it), `[table]` and `[[array of
  tables]]` headers, inline tables, and arrays over several lines with
  comments and a trailing comma. Defining a key or a table twice, adding
  to an inline table, and mixing a table with an array of tables are
  errors with their line.
- `to_json` gives the JSON text, for code that wants to read a document
  without a struct (`json.decode[Map[Str, ...]]`) or print it.

