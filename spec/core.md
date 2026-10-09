# Xo Language Specification (Core)

Status: draft 0.7 (2026-10-08)
Edition: 2026
Companion: `spec/std.md` (prelude methods and standard library surface)

Xo is a small, statically typed, garbage collected language for backends and
web services. It keeps what makes Go productive (few keywords, one obvious way,
fast builds, static binaries, a strong standard library, a compatibility
promise) and is designed so that an AI agent can write, check, build, and debug
programs for any OS with fast, structured, unambiguous feedback.

## Changes in 0.7

Decisions 0041 (Go packages) and 0042 (two phase shutdown).

- Request scoped arenas are on by default for HTTP handlers (5.5; decision
  0066): an implementation detail with no observable effect, off with
  `--no-arenas`.

- Go packages with `use go` (4.4).
- Two phase shutdown (7.5).
- `Mutex.with_read`, shared read only access (5.4, std.md 2.7; decision
  0063). A closure parameter declared `var` where the expected function
  type takes it by value is error `XO0401` (it changed a copy only).
- `BytesBuilder`, the move only builder of `Bytes` (5.3, std.md 2.7;
  decision 0078).
- Warning `XO0305` is not reported for a `_` arm that hides at least 8
  variants and more than half of the enum (3.5; decision 0080).
- A rune literal where an integer type is expected is its code point
  (1.6); `Int(r)`, `Rune(n)`, and `Rune.try(n)` convert between `Rune` and
  the integer types (2.1; decision 0079).
- `xo build --hybrid` compiles hot leaf loops to native assembly inside Go
  backend builds (10.1; decision 0084).
- The native backends check `ensures` and struct invariants and unwind
  faults (deferred code, scopes, Mutexes, `task.isolate`, a 500 from an
  HTTP handler) as the Go backend does (3.5, 8, 9.1; decision 0085).

## Changes in 0.6

Driven by the independent replant of planted safety bugs
(`bench/results/planted-xo-2026-10-07.md`). Decision 0038.

- Reading a `Map` by index, or changing an element in place through it, is
  error `XO0410`; use `get`, `get_or_fault`, or `update`. `m[k] = v` stays (3.6).
- Warning `XO0305`: a `_` arm hides variants of an enum declared in the same
  module (3.5).
- Warning `XO0804`: a value read in one `Mutex.with` is used, or acted on, in a
  later `with` on the same mutex (5.4).
- Regression tests for shrunk contract fuzz failures go in the module's
  `regressions_test.xo`, not `testdata/regressions/` (9.2).
- Go packages with `use go` (4.4), with the `go` effect.
- `for x in ch` over channels (3.5, 7.3); `task.Once` and
  `task.map_concurrent_until` in std.
- Two phase shutdown: the first signal requests a stop (`task.stop_requested`,
  `task.stopping`), cancellation follows after a grace period (7.5).
- Std: `Map.get_or_fault`, hex and base64 on `Bytes`, `Time.from_unix_millis`,
  `Atomic.max`, `Range.to_list`, `Patch.apply_to`/`apply_opt`, and `std/path`.

## Changes in 0.5

Driven by the stage 2 benchmark (`bench/results/stage2-xo-check-2026-10-07.md`).
Decision 0037.

- XO0520: a mutated copy of a collection element must be stored again; reading
  it afterwards no longer counts (5.2).
- `is` may be combined with `&&` and `||` (3.5, 15).
- Each arm of a tail `if` or `match` is in return position (2.6).
- `testing` is usable in helper signatures in `_test.xo` files (9.2).

## Changes in 0.4

Driven by the P1 parser and checker, the planted bug benchmark, and the harder
tasks (`bench/results/planted-and-harder-2026-10-07.md`,
`bench/results/check-corpus-v0.3.md`). Decisions 0028 to 0035.

- Task groups that can be stored in structs, `os.tasks` for program lifetime
  work, `try_send`/`try_recv`, `task.map_concurrent` (7.1, std).
- Discarding a `Result` with `let _ =` is an error; use
  `.ignore_err("reason")` (2.6).
- Effect arguments of generic types may be omitted (6.2).
- `x is Pattern` tests a pattern (3.5). All compound assignments, tuple
  fields `t.0`, local `const`, qualified types in `from`, doc comments on
  fields and variants.
- XO0520 also covers `.or_err(...)?` copies and values mutated after being
  stored in a collection (5.2).
- `Mutex.with` keep and fail pattern documented (5.4).
- Grammar readings from the parser and typing readings from the checker are
  written into the relevant sections (1.3, 1.6, 2.1, 2.6, 2.10, 3.3, 7.1, 15).
- Streaming HTTP bodies, `Fs.write_only`, fuller `Set`, `testing.run_main`
  (std).

## Changes in 0.3

Driven by the second paper test (`bench/results/paper-2026-10-07-v0.2.md`).
Decisions 0021 to 0027.

- String interpolation is `${expr}`; `{` and `}` are literal in plain strings.
  Raw strings `r#"..."#` may contain quotes (1.6).
- `defer` blocks are shielded from cancellation (3.5, 7.2).
- XO0520 applies only to values copied out of a collection (5.2).
- `?` works on `Option` in functions returning an optional (2.6).
- A line starting with `.` continues the previous line (1.3).
- Implicit conversions are listed in one place: there are three (2.10).
- Block and loop typing, generic literal inference, qualified variant
  patterns, module local names, pure means no capability effects, `uses (...)`
  allowed on declarations, process signals (various sections).

## Changes in 0.2

Driven by the paper test in `bench/results/paper-2026-10-07.md`. Decisions
0011 to 0020 in `knowledgebase/decisions/`.

- Explicit failure with `return Err(e)` and tail `Err(e)` (2.6). New `Never`
  type (2.1).
- `Ok Err Some None` replace `ok err some none`. `result` and `old` are
  contextual inside `ensures` only (1.4, 2.5, 2.6).
- Full pattern syntax including `_`, literals, ranges, or patterns, nested
  patterns, positional and named variant fields (3.5).
- Unused binding rule relaxed for parameters and `_` names (3.1).
- Effect variables and effect subsumption (6.2). Multiple effects in a
  function type are parenthesized.
- Scope rules: background tasks, exit behavior, error propagation; `scope ...
  timeout` removed in favor of `task.with_timeout` (7).
- Cancellation is observable as a `Canceled` error from fallible blocking calls
  (7.2).
- `Mutex.with` signature and write back rule (5.4); lost update check `XO0520`
  and `Map.update` (5.2).
- Test blocks get an implicit `os: TestOs`; `std/test` renamed `std/testing` (9.2).
- Logging is ambient and untracked (6.1).
- Slicing with ranges, closure literal grammar, trailing commas, const struct
  literals, `{}` is always an empty map (1.6, 3.x, 15).

## 0. Design goals

1. **No guessing.** Every name resolves to exactly one definition, found by
   reading local code. No overloading, implicit conversions, inheritance,
   macros, `init()`, or hidden control flow.
2. **The compiler is an API.** All tool output is structured JSON with stable
   codes and machine-applicable fixes.
3. **Errors cannot be ignored, nil does not exist, matches are exhaustive.**
4. **Authority is explicit.** Side effects are declared in signatures and
   performed only through capability values passed down from `main`.
5. **Concurrency is structured.** No task outlives its scope. No shared
   mutable state without an explicit synchronized type.
6. **Every target is checked on every build.** No build tags.
7. **Every failure is reproducible.** Effects are recordable and replayable.
8. **Compatibility.** Code that builds under an edition keeps building under
   that edition forever. New editions ship with automatic rewriters.

### Staging

Each section is tagged with the phase in which it is implemented.

- **[P1]** Prototype: `xo` frontend written in Go, emits Go source, uses the Go
  toolchain for codegen, GC, scheduler, and cross compilation.
- **[P2]** Own runtime: deterministic scheduler, full record/replay, task tree.
- **[P3]** Own backend and content addressed code store.

The language semantics do not change between phases. Only implementation
quality and tooling depth change.

### Experimental sections

None. The five rules that were experimental through draft 0.6 (value
semantics, required `uses` on `pub` functions, no module declaration, block
scoped `defer`, enforced naming with `pub`) were accepted on 2026-10-08 from
the benchmark evidence (decisions 0003, 0004, 0006, 0009, 0010).

---

## 1. Lexical structure [P1]

### 1.1 Source files

- Encoding: UTF-8. Extension: `.xo`.
- A **module** is a directory. All `.xo` files in a directory belong to the
  same module and share one namespace. There is no module declaration in the
  file: the directory path is the module identity.
- `use` declarations are per file, as in Go.
- Files ending in `_test.xo` contain only tests and test helpers.

### 1.2 Comments

```
// line comment
/// doc comment, attaches to the next declaration (Markdown)
```

There are no block comments. Fenced `xo` code blocks inside doc comments are
rendered by `xo doc`; compiling and running them as examples under `xo test`
is planned ([P2]).

### 1.3 Statement termination

Newlines terminate statements, using Go's rule: a newline ends a statement if
the last token on the line is an identifier, literal, `)`, `]`, `}`, `?`, or one
of `return break continue`. Semicolons are not written by programmers and are
removed by `xo fmt`.

Exception: a line whose first token is `.` continues the previous line, so
method chains can be split:

```
http.json(429, {"error": "rate limited"})
  .with_header("Retry-After", "${secs}")
```

A line whose first token is a declaration clause (`uses`, `requires`,
`ensures`, `invariant`, `derive`), the `->` of a result type, `else` after a
`requires`, or the `{` that opens a function body, also continues the
declaration above it.

Consequences: `} else {` and `} else if` must be on the same line as the
closing brace. A list that spans lines needs a trailing comma after the last
element. Trailing commas are allowed in every comma separated list.

### 1.4 Keywords (28)

```
as        break     const     continue  defer     else      ensures
enum      fn        for       if        in        interface invariant
let       match     pub       requires  return    scope     select
struct    test      type      use       uses      var       while
```

Predeclared identifiers (cannot be redefined at top level):

- Values and functions: `true false self target fault dbg`
- Variants: `Ok Err` (of `Result`), `Some None` (of `Option`), usable without a
  type prefix everywhere.
- Types: those in section 2.1 and the prelude interfaces in `spec/std.md`.
- In test blocks only: `os`, `expect`, `testing`.

Contextual words (meaningful only in one position, usable as identifiers
elsewhere): `derive` (after a type body), `from` (in an enum variant),
`with` (postfix struct update), `after` (in a `select` arm), `seed` (after a
test name), `result` and `old` (inside `ensures`), `is` (binary operator
position).

To keep parsing unambiguous, an unparenthesized struct literal may not appear
in an `if`, `while`, `for`, `match`, `requires`, or `ensures` expression (the
same rule as Go). Write `if (p == Point{x: 0, y: 0}) { ... }`.

### 1.5 Identifiers and naming

- Identifiers: `[A-Za-z_][A-Za-z0-9_]*`.
- Naming is enforced by the compiler (error `XO0101`), not left to linters:
  - Types, enum variants, interfaces: `UpperCamel`.
  - Functions, methods, variables, fields, modules: `snake_case`.
  - Constants: `UPPER_SNAKE`.
  - Effect variables: `snake_case`.
- Exported names are marked with `pub`, not by capitalization.

### 1.6 Literals

```
42  1_000_000  0xff  0b1010  0o755        // Int
3.14  1e-9                                // Float
true  false                               // Bool
'a'  '\n'  '\u{1F600}'                    // Rune
"hello ${name}, you have ${count + 1} items" // Str with interpolation
"GET /users/{id}"   "{\"a\": 1}"           // braces are literal
"price: \$5"                              // \$ is a literal $ before {
r"raw: no escapes, no interpolation \d+"
r#"raw, may contain "quotes": {"a": 1}"#
"""
multi line string, common leading indentation is removed
"""
b"raw bytes\x00"                          // Bytes, escapes but no interpolation
5s  250ms  10us  2m  1h                   // Duration
[1, 2, 3]   []                            // List
{"a": 1, "b": 2}   {}                     // Map
#{1, 2, 3}   #{}                          // Set
```

- `${expr}` interpolates any expression whose type implements `Display`.
  `Secret[T]` values render as `<redacted>`. `$` not followed by `{` is literal.
- Raw strings `r"..."` have no escapes and no interpolation. `r#"..."#` may
  also contain `"`, which makes JSON fixtures easy to write.
- An integer literal takes the integer type expected in context (`Int32`,
  `UInt8`, ...), defaulting to `Int`. It never becomes a `Float`; write `2.0`.
- A rune literal is a `Rune`, except where an integer type is expected (an
  annotation, a parameter, the other operand of an operator, a pattern on
  an integer, a `const` of an integer type): there it is that type's value
  of its code point, so byte level code reads `b[i] == '"'` and
  `match c { 'a'..='z' => ... }` on a `UInt8` (decision 0079). The code
  point must fit the type; `Int8` and `UInt8` take only ASCII (`'é'` is two
  bytes in UTF-8, so it is never one byte): error `XO0401` otherwise.
- Duration literals are an integer followed by `ns`, `us`, `ms`, `s`, `m`,
  or `h`. `1.5s` is not a literal; write `1500ms`.
- Escapes in `Str` and `Bytes`: `\n \t \r \0 \\ \" \' \$ \xHH \u{...}`.
  In `Str`, `\xHH` is at most `\x7f`.
- `{}` in expression position is always an empty `Map`. An empty block used as
  a value is written `()`.

---

## 2. Types [P1]

### 2.1 Builtin types

| Type | Description |
|---|---|
| `Bool` | `true` or `false` |
| `Int` | 64 bit signed. Overflow faults. |
| `Int8 Int16 Int32 Int64` | Sized signed integers. Overflow faults. |
| `UInt8 UInt16 UInt32 UInt64` | Sized unsigned integers. Overflow faults. |
| `Float` | 64 bit IEEE 754. `Float32` also exists. |
| `Rune` | Unicode scalar value |
| `Str` | Immutable UTF-8 text |
| `Bytes` | Immutable byte sequence |
| `Duration`, `Time` | Monotonic duration and wall clock instant |
| `List[T]` | Immutable persistent vector |
| `Map[K, V]` | Immutable persistent hash map, insertion ordered |
| `Set[T]` | Immutable persistent hash set, insertion ordered |
| `Buf[T]` | Mutable, move only growable buffer (section 5.3) |
| `Chan[T]` | Channel handle (section 7) |
| `Mutex[T]`, `Atomic[T]` | Shared mutable state (section 5.4) |
| `Secret[T]` | Value that is redacted in every output path |
| `Option[T]`, written `T?` | Optional (section 2.5) |
| `Result[T, E]` | Success or failure value (section 2.6) |
| `(A, B)` | Tuple |
| `fn(A) -> B ! E uses fx` | Function type (section 6.2) |
| `()` | Unit |
| `Never` | The type with no values. Type of `return`, `break`, `continue`, `fault(...)`, `proc.exit(...)`, and infinite loops. Converts to every type. |

There are no implicit numeric conversions. Conversions are calls on the target
type: `Float(i)`, `Int(f)` (truncates toward zero, faults on NaN or out of
range), `Int32(x)` (faults out of range), `Int32.try(x) -> Int32?`. Parsing
text is `Int.parse(s) -> Int?`, `Float.parse(s) -> Float?`.

A `Rune` is not a number, but converts both ways with an integer type
(decision 0079): `Int(r)` (and every sized integer type, faulting out of
range: `UInt8('é')` faults) is its code point; `Rune(n)` from any integer
type is the character with code point `n` and faults when `n` is not a
Unicode scalar value (negative, a surrogate `0xd800` to `0xdfff`, or above
`0x10ffff`); `Rune.try(n) -> Rune?` is `None` there instead. A `Rune`
does not convert to `Float` (`XO0409`, with a note pointing at `Int(r)`).

Float comparison (decision 0053; the same for `Float32`):

- `==` and `!=` are IEEE 754: NaN is not equal to anything, itself
  included, and `-0.0 == 0.0`. Structural `==` on structs, tuples, enums,
  options, lists, and recursive (boxed) values compares Float fields this
  way, so a value holding a NaN is not equal to itself (`x == x` is
  false).
- `<`, `<=`, `>`, `>=` use the total order of `Ord` (std.md 1): every
  NaN sorts after `+Inf` and equals every other NaN in the order, and
  `-0.0` and `0.0` are equal in the order. So `NaN < NaN` is false,
  `NaN <= NaN` is true, `NaN > x` is true for every non NaN `x`,
  `-0.0 < 0.0` is false, and `!(a < b)` is always `a >= b`. Because the
  two families differ, `a <= b && a >= b` does not imply `a == b` (both
  NaN).
- Unary `-` flips the sign of every Float, zero included: `-0.0` is
  negative zero.

`Duration` may be negative (`d.abs()` gives the magnitude).

Operators on `Duration` and `Time`:

| Expression | Type |
|---|---|
| `d1 + d2`, `d1 - d2` | `Duration` |
| `d * n`, `n * d`, `d / n` (n: `Int`) | `Duration` |
| `d1 / d2` | `Int` (truncated) |
| `t + d`, `t - d` | `Time` |
| `t1 - t2` | `Duration` |
| comparisons | `Bool` (both are `Ord`) |

Methods (`d.seconds() -> Float`, `d.scale(f: Float) -> Duration`, and more) are
listed in `spec/std.md`.

### 2.2 Structs

```
pub struct User {
  pub id: Int
  pub name: Str
  pub email: Str?
  created: Time
}
```

- Construction names every field: `User{id: 1, name: "a", email: None, created: now}`.
  Type arguments of a generic struct are inferred from the fields or the
  expected type: `Job{id: 1, input: "x"}` is a `Job[Str]`.
  Fields with a declared default may be omitted:
  `struct Config { port: Int = 8080 }`, so `Config{}` is valid.
- Update syntax creates a modified copy: `u with {name: "b"}`.
- Structs are values. Assignment copies (cheaply: fields are shared because all
  builtin collections are immutable).
- Fields may be separated by newlines or commas.

### 2.3 Enums (sum types)

```
pub enum LoadErr {
  NotFound(id: Int)
  Db(msg: Str)
  Timeout
}
```

- Variants may carry named fields. Construction uses names:
  `LoadErr.NotFound(id: 42)`. A single field variant may also be constructed
  positionally: `LoadErr.NotFound(42)`.
- The enum prefix may be omitted when the expected type is known (in a
  `match` arm, an argument, a return, a typed binding, or nested inside one of
  those): `return Err(NotFound(id: 42))`.
- Enums from other modules are qualified with the module: `task.Timed.TimedOut`,
  `http.ClientErr.Canceled`. Type arguments of generic enums are inferred:
  `task.Timed.Failed(e)`.
- Enums are closed. `match` must be exhaustive (error `XO0301`). Variants with
  a field of type `Never` cannot be constructed and may be omitted from a
  `match`.

### 2.4 Interfaces

```
pub interface Store {
  fn get(self, id: Int) -> User ! LoadErr uses net
  fn put(self, u: User) -> () ! LoadErr uses net
}
```

- Satisfied structurally and implicitly, as in Go.
- Interfaces may be used as types (dynamic dispatch) or as generic constraints.
- `Never` satisfies every interface.
- A type satisfies an interface through its methods whether or not they are
  `pub`. Calling a non `pub` method by name from another module is still an
  error.
- Every derivable name (`Display`, `Ord`, `Json`, ...) is also an interface and
  may be used as a constraint: `fn reply[T: Json](v: T)`.

### 2.5 Optionals

- `T?` is `Option[T]`: either `Some(v)` or `None`. There is no nil, null, or
  zero value pointer.
- A plain `T` is accepted where `T?` is expected and is wrapped in `Some`
  automatically (one of the three implicit conversions, section 2.10).
- Unwrapping:
  ```
  if let Some(e) = user.email { send(e) } else { log.warn("no email") }
  let e = user.email ?? "unknown@example.com"
  let e = user.email.or_err(LoadErr.NotFound(id: user.id))?
  ```
  `if let e = opt` (without `Some`) is also accepted as shorthand for
  `if let Some(e) = opt`.
- In a function that returns `U?`, `opt?` yields the value or returns `None`:
  ```
  fn retry_after(resp: http.Response) -> Duration? {
    let secs = Int.parse(resp.header("Retry-After")?.trim())?
    1s * secs
  }
  ```
  Using `?` on an `Option` in a function that returns a `Result` is error
  `XO0204`, with a fix that inserts `.or_err(...)`.

### 2.6 Results, errors, and explicit failure

- A function that can fail declares its error type after `!`:
  `fn load(id: Int) -> User ! LoadErr`. Such a function returns
  `Result[User, LoadErr]` to its caller.
- `!` with no type means `! Error`, the builtin open error interface.
  `fn main(os: Os) !` is `fn main(os: Os) -> () ! Error`.
- `T ! Never` is the same type as `T`: a function that cannot fail.
- **Returning.** In a function declared `-> T ! E`, a returned value (a tail
  expression or `return x`) may be:
  - a `T`, which means success, or
  - `Err(e)` with `e: E`, which means failure, or
  - any `Result[T, E]` value, returned as is.

  ```
  fn parse_port(s: Str) -> Int ! ConfigErr {
    let n = Int.parse(s).or_err(ConfigErr.NotANumber(s))?
    if n < 1 || n > 65535 {
      return Err(ConfigErr.OutOfRange(n))
    }
    n
  }
  ```
  If `T` is itself a `Result`, success must be written `Ok(x)` explicitly
  (error `XO0203` otherwise).
- Each arm of a tail `if` or `match` is in return position, so arms may mix
  `T` values and `Err(e)`: `match x { 0 => 1  _ => Err("no") }`.
- **Propagating.** `expr?` on a `Result[T, E2]` yields the `T` or returns the
  error from the enclosing function. `E2` must convert to the enclosing error
  type `E`:
  - `E2` is `E`, or
  - `E` is `Error` and `E2` implements `Display`, or
  - `E` has a `from E2` variant.
- **from variants** wrap other error types:
  ```
  enum ApiErr {
    BadRequest(msg: Str)
    Load(from LoadErr)   // `?` on a `! LoadErr` call wraps into ApiErr.Load
  }
  ```
  A `from` variant has one positional field: construct with
  `ApiErr.Load(LoadErr.Timeout)`, match with `Load(inner)`.
- **Mapping.** `r.map_err(f)` converts the error inline:
  `json.decode[User](b).map_err(fn(e) { ApiErr.BadRequest(msg: "${e}") })?`.
- **Handling is mandatory.** A `Result` value that is never used (not
  propagated with `?`, matched, passed, or returned) is error `XO0201`. A
  `Result` bound to a name that is never used is also `XO0201`, whatever the
  name (including `_x`).
- **Discarding needs a reason.** `let _ = expr` on a `Result` is error
  `XO0206`. Write `expr.ignore_err("why this failure does not matter")`,
  which yields `T?` and records the reason, so every discarded error is
  searchable and reviewable:
  ```
  let _ = dst.remove(tmp).ignore_err("best effort temp file cleanup")
  let nick = nickname_field(fields).ignore_err("nickname is optional") ?? ""
  ```
  `r.ok()` remains for when failure simply means "no value".
- A closure whose tail is a `Result` is a failing closure; the `Result` is
  returned as is. A closure declared `-> T` with no `!` cannot fail.
- A non `Result` value may be discarded as an expression statement.

### 2.7 Generics

```
pub fn map[T, U](xs: List[T], f: fn(T) -> U) -> List[U] { ... }
pub fn largest[T: Ord](a: T, b: T) -> T { if a > b { a } else { b } }
```

- Square brackets, Go style. Constraints are interfaces. Effect variables are
  declared in the same list (section 6.2).
- Explicit type arguments at a call: `json.decode[User](body)`.
- Monomorphized at compile time. Type arguments are inferred at call sites when
  unambiguous; otherwise error `XO0402` lists the candidates.
- No higher kinded types, no specialization, no variadic generics.

### 2.8 Type aliases and newtypes

```
type UserId = Int                   // alias, interchangeable with Int
type Email struct(Str)              // newtype, distinct, construct with Email("x")
type Handler = fn(http.Request) -> http.Response ! Error uses net
```

### 2.9 Derived behavior

Every type automatically implements:

- `Debug`: structural printing used by the debugger, error frames, and `dbg()`.
- `Eq` and `Hash` when all fields do.

Opt in with `derive` on the declaration:

```
pub struct User { ... } derive Display, Ord, Json, OpenApi
```

- Derived `Display` prints the same text as `Debug`:
  `NotFound(id: 42)`, `User{id: 1, name: "a"}`. Implement `Display` by hand
  (`fn (e: ApiErr) display() -> Str`) for user facing text.
- `Json`, `OpenApi`, and `Proto` derives generate codecs and schemas from the
  same declaration so wire formats and docs cannot drift. JSON rules are in
  `spec/std.md`.
- `SqlRow` (structs only) generates a decoder from a std/sql row: each field
  is read from the column of the same name (`spec/std.md` 10.4).

### 2.10 Implicit conversions

There are exactly three, and each applies only where the target type is
already known:

1. A `T` where a `T?` is expected becomes `Some(T)`.
2. A value where an interface type is expected (a parameter, a field, a
   collection element, a return) becomes that interface, if the type
   implements it. This includes passing a capability fake where the
   capability interface is expected.
3. An error at a `?` site converts to the enclosing error type (2.6).

Conversions are not applied when joining branches (`if`/`match` arms must
already agree) or to the `else` value of `requires`. Comparing `opt == v`
wraps `v` in `Some`.

Everything else, including all numeric conversions, is explicit.

---

## 3. Declarations and statements [P1]

### 3.1 Bindings

```
let x = 10            // immutable binding
var total = 0         // mutable binding
total += x
const MAX_USERS = 1000
const DEFAULT = Config{port: 8080}
```

- `const` may appear at top level or in a block. Initializers must be compile
  time values: literals, other
  constants, struct and enum construction, and collection literals of those.
- Unused local bindings, local constants, and imports are errors (`XO0110`). Not covered:
  function and closure parameters, names starting with `_`, and `_` itself.
  `xo fix` removes unused bindings.
- Shadowing in the same block is an error. Shadowing in a nested block is
  allowed.

### 3.2 Functions

```
/// Returns the user with the given id.
pub fn fetch_user(db: Db, id: Int) -> User ! LoadErr
  uses net
  requires id > 0
{
  let row = db.query_one("select id, name, email from users where id = $1", id)?
  User{id: row.int("id")?, name: row.str("name")?, email: row.str_opt("email")?, created: row.time("created")?}
}
```

- The last expression of a block is its value. `return` exits early. An empty
  body `{}` has type `()`.
- A block whose last item is a statement (an assignment, `let`, `for`,
  `while cond`, `defer`) has type `()`.
- `while { ... }` with no `break` has type `Never`, so it may end a function
  body of any type (all exits are `return`).
- Parameters are immutable unless declared `var` (in out, see 3.3). Named
  arguments are allowed and recommended when a call has more than two
  parameters of the same type: `transfer(from: a, to: b, amt: 5)`. Positional
  arguments must come before named ones.
- Default parameter values are allowed: `fn listen(addr: Str, backlog: Int = 128)`.
- No variadic functions. Pass a `List` or `Map`.

### 3.3 Closures

```
let double = fn(x: Int) -> Int { x * 2 }
xs.map(fn(x) { x * 2 })                         // types inferred from context
srv.handle("GET /hi", fn(req) { http.text(200, "hi") })
let load = fn(id: Int) -> User ! LoadErr uses net { store.get(id)? }
```

- Parameter and return types may be omitted when the expected function type
  is known.
- Closures capture by value. Mutating a captured `var` inside a closure is
  error `XO0502`. To share mutable state with a closure, capture a `Mutex` or
  `Atomic` handle.
- Creating a closure performs no effects; calling it does.
- `return` inside a closure returns from the closure.
- Effects of a closure are inferred from its body unless declared.

### 3.4 Methods

```
fn (u: User) display_name() -> Str { u.name }

pub fn (var a: Account) deposit(amt: Money) requires amt > 0 {
  a.balance += amt
}
```

- A `var` receiver mutates the caller's binding in place (in out semantics). It
  can only be called on a `var` binding (error `XO0501`), a `var` parameter, or
  a field of one (`st.users.put(k, v)` where `st` is `var`).
- Methods may be declared only in the module that declares the type.

### 3.5 Control flow and patterns

```
if cond { ... } else if other { ... } else { ... }   // also an expression

for x in xs { ... }
for (i, x) in xs.indexed() { ... }
for (k, v) in m { ... }
for job in jobs_chan { ... }  // receives until the channel is closed and drained
for i in 0..10 { ... }        // half open range, 0..=10 is inclusive
for _ in 0..3 { ... }
while cond { ... }
while { ... }                 // infinite loop

match code {
  200..=299 => "ok"
  429 | 503 => "retry later"
  _ => "error"
}
```

- `if` without `else` has type `()`.
- `x is Pattern` is a `Bool` expression that tests a pattern without binding
  names: `expect(r is Err(Timeout))`, `if resp is Ok(_) && retries < 3`.
  `is` is a contextual word in operator position (precedence of `==`), so it
  combines with `&&` and `||` like a comparison.
- Compound assignment works for `+= -= *= /= %=` on `var` bindings and their
  fields. There are no bitwise compound assignments.
- A value counts as stored again (for XO0520) by any later `push`, `put`,
  `insert`, `set`, `add`, `update`, or `m[k] = v` of it, including later in
  the same loop.
- Tuple fields are `t.0`, `t.1`, ...
- `match` is an expression. Arms are separated by newlines. An arm body may be
  an expression, a block, or a single statement such as an assignment (type
  `()`): `Timeout => last = Some(AttemptErr.Timeout)`.
- Labeled loops: `outer: for ... { break outer }`.
- `defer stmt` or `defer { ... }` runs when the enclosing **block** exits (not
  the function, unlike Go), in reverse order, including on errors, faults, and
  cancellation. Values are read when the deferred code runs.
- Deferred code is **shielded from cancellation**: inside it, cancellation
  points do not report cancellation, so cleanup such as removing a temp file
  works after a timeout. A shielded `defer` that runs longer than 5 seconds
  faults (`XO-F005`).
- The native backends (`--backend=llvm`, `--backend=arm64`) run deferred
  code on every way out alike, faults included (decisions 0070, 0085).

**Patterns** are used in `match`, `let`, `if let`, and `for`:

| Pattern | Matches |
|---|---|
| `_` | anything, binds nothing |
| `x` | anything, binds `x` |
| `42`, `"GET"`, `'a'`, `true` | equal literal |
| `1..=9`, `'a'..='z'` | value in inclusive range |
| `p1 \| p2` | either (both must bind the same names) |
| `(a, b)` | tuple |
| `Some(p)`, `None`, `Ok(p)`, `Err(p)` | prelude variants |
| `Timeout`, `http.ClientErr.Canceled` | unit variant, bare or qualified |
| `NotFound(p)` | variant fields positionally, in declared order |
| `NotFound(id: p)` | variant fields by name; `..` ignores the rest: `Db(msg: m, ..)` |
| `Variant(..)` | variant with any fields |
| `User{id: p, ..}` | struct fields by name |
| `[]`, `[first, ..]`, `[.., last]`, `[a, .., z]` | list shape (`..` at most once, anywhere) |
| `(p)`, `()` | grouping, unit |

Patterns nest: `Some(Ok(v))`, `Err(LoadErr.NotFound(id))`.

An unguarded `_` (or catch all binding) arm that hides variants of an enum
declared in the same module is warning `XO0305`: list the variants, so that
adding one later is a compile error at every `match`. Enums from other modules
may use `_`, since their owners can add variants. A `_` arm that hides at
least 8 variants and more than half of the enum is a default ("these few are
special, the rest are not", as on a token kind enum) and is not reported; the
warning names at most 8 hidden variants, then how many more (decision 0080).
An `UpperCamel` name in a pattern always refers to a variant; bindings are
`snake_case` (1.5), so the two never collide.
`let` requires an irrefutable pattern (`let (a, b) = pair`).

### 3.6 Indexing and slicing

- `xs[i]` on `List`, `Bytes`: faults when out of range. `xs.get(i)` returns `T?`.
- `Map` cannot be read by index: `m[k]` in an expression, or an in place change
  through it (`m[k].f = v`, `m[k] += 1`, `m[k].push(x)`), is error `XO0410`,
  because a missing key is the Map form of a nil dereference. Use `m.get(k)`
  (`V?`), `m.get_or_fault(k, "why the key must exist")`, or
  `m.update(k, init, fn(var v) { ... })`. On a `var` map, the whole value
  assignment `m[k] = v` is allowed and means `m.put(k, v)`.
- `Str` cannot be indexed by position (use `runes()` or slicing).
- Ranges slice: `xs[1..]`, `xs[..n]`, `xs[a..b]` on `List`, `Bytes`, and `Str`
  (byte offsets; slicing a `Str` inside a UTF-8 sequence faults). Slices of
  immutable values are O(1) and share storage.

### 3.7 Holes

`???` is a typed hole, valid anywhere an expression is expected.

- `xo check` succeeds with warnings and reports, for each hole, the expected
  type, the effects in scope, and in scope values or functions that fit
  (diagnostic `XO0900`, see section 10).
- `xo build` rejects programs with holes. `xo run --holes` and `xo test --holes`
  allow them and fault if a hole is evaluated.

Holes let an agent write a type correct skeleton first and fill it in steps.

---

## 4. Modules and dependencies [P1, content addressing P3]

### 4.1 Imports

```
use std/http
use std/json as j
use github.com/acme/billing/invoice
```

- The last path segment is the local name unless `as` is given, with `-`
  replaced by `_` (`use acme/rate-limit` binds `rate_limit`). If the result is
  still not a valid identifier, or is a keyword, `as` is required (`XO0602`).
- Imports are explicit. There is no dot import or glob import.
- Cyclic module imports are an error.

### 4.2 xo.mod

```
module github.com/acme/api
edition 2026

require github.com/acme/billing v1.4.0
```

**[P1]** `gc` lines tune the garbage collector of a program whose main
module this is (decision 0065); they are ignored in imported modules:

```
gc min_heap 64MiB      // floor of the heap goal
gc percent 200         // GOGC: goal growth over the live heap, or `off`
gc memory_limit 1GiB   // soft memory limit (GOMEMLIMIT)
```

Sizes take `B`, `KiB`, `MiB`, `GiB`, `TiB` (or `KB`, `MB`, `GB`, `TB`, powers
of 1000). Without `gc` lines a program's heap goal is the live heap plus 100
percent of it (as Go's GOGC=100), but never below a floor of 2 MiB per
processor (`GOMAXPROCS`), at least 16 MiB; there is no memory limit. The
environment always wins: `GOGC` set turns the floor off and sets the
percent, `GOMEMLIMIT` replaces `memory_limit`, `XO_GC_MIN_HEAP` replaces
`min_heap` (0 turns the floor off).

### 4.3 xo.lock

Written only by the toolchain. Maps every imported definition actually used to
its content hash.

- **[P1]** Hash is per module version (source tree hash), like Go's `go.sum`.
- **[P3]** Hash is per definition: the normalized AST with local names erased
  and referenced definitions replaced by their hashes. Consequences:
  - Two versions of a dependency coexist without conflict. Types from different
    versions are the same type if and only if their hashes are equal.
  - An upgrade that does not change a definition is a no-op for code using it.
  - A name that does not resolve to a hash fails at once with the nearest real
    names suggested (`XO0601`). APIs cannot be hallucinated silently.

### 4.4 Go packages [P1]

Until Xo has its own ecosystem, Go packages can be used directly (decision
0041):

```
use go "regexp"
use go "github.com/google/uuid" as uuid

fn main(os: Os) ! {
  let re = regexp.must_compile("[a-z]+")
  os.stdio.println("${re.find_string("123abc")} ${uuid.new_string()}")
}
```

- `xo check`, `build`, `run`, and `test` generate bindings automatically into
  `.xo/gobind/` (or `$XO_GOBIND_DIR`); `xo bind <path>` does it explicitly.
  Third party module versions follow Go's module resolution.
- Names are converted to Xo naming: functions and methods to `snake_case`
  (`Split` is `split`, `EncodeToString` is `encode_to_string`), types stay
  `UpperCamel`.
- Types at the boundary: `(T, error)` becomes `T ! go.Error`; a pointer result
  that may be nil becomes `T?`; mutable Go objects become opaque handles;
  slices and maps are copied in and out; a `context.Context` parameter is
  supplied from the current task (deadline and cancellation); a panic becomes
  a fault.
- An Xo function passed to Go as a callback that Go calls during the Go call,
  on the calling task's goroutine, runs as the calling task. A call from
  another goroutine (`sync.WaitGroup.go_`, a timer) runs as a task of its own,
  a child of the calling task (canceled with it), with its own held mutexes
  (decision 0067); under the deterministic scheduler such a call waits for
  a turn like a spawned task and is recorded by `--record` (core 7.4, 11.3).
- Every call into Go performs the effect `go`, which `pub` functions must
  declare. Go code can reach files and the network without a capability, so
  `uses go` marks that authority escape in signatures.
- Exported names whose signatures cannot be mapped are skipped and listed at
  the top of the generated binding; using one is `XO1102`. A package that
  cannot be bound is `XO1101`.

---

## 5. Values, mutation, and memory [P1]

### 5.1 Value semantics

All builtin types and user structs, enums, and tuples have value semantics.
There are no pointers and no reference types other than the explicit handle
types (`Chan`, `Mutex`, `Atomic`, capability values, `Buf` by move).

Because values never alias mutably, data races are impossible except through
`Mutex` and `Atomic`, which are synchronized.

### 5.2 Mutation

- Only `var` bindings, `var` parameters, and `var` receivers can be mutated.
- `xs.push(v)` on a `var xs: List[T]` rebinds `xs` to a new persistent list
  sharing structure with the old one. The same holds for every mutating
  collection method (`put`, `remove`, `update`, `pop`, `add`).
- Field assignment `u.name = "b"` on a `var u` rebinds `u` to a modified copy.
  Nested paths work: `st.users.put(k, v)`, `st.config.port = 9090`.
- To change a value stored in a collection, use `update`, which reads, mutates,
  and writes back in one call:
  ```
  let allowed = m.update(key, new_bucket(limit, now), fn(var b) { b.take(now) })
  ```
- **Lost update check.** Error `XO0520` ("mutation of `b` is never written
  back") applies to a local `var` whose initializer copies an element out of a
  collection (`m.get(k)`, `m[k]`, `xs[i]`, `xs.get(i)`, `first`, `last`,
  `find`, optionally followed by `??`, `?`, or `.or_err(...)?`) when the
  binding is mutated and is not stored again after its last mutation. Storing
  means passing the whole value as a call argument (`m.put(k, b)`,
  `save(b)`) or assigning it as a whole (`m[k] = b`). Reading or returning it
  does not count, because the collection still holds the old value. This
  catches `var b = m.get(k) ?? new_bucket(); b.take(now)` without
  `m.put(k, b)`, and a PATCH handler that returns the edited copy but never
  stores it. For an intentional scratch copy, copy through a local first
  (`let base = m.get(k)?; var scratch = base`). Values derived with `with`
  are not tracked, since views such as redacted copies are common. It also applies to any local `var` that is mutated after
  being stored into a collection (`xs.push(v)`, `m.put(k, v)`) and then not
  stored again, since the stored copy does not see the change. Reads inside
  `defer` count. Parameters, including `var` closure parameters, are never
  checked.

### 5.3 Move only buffers

`Buf[T]`, `StrBuilder`, and `BytesBuilder` are mutable in place and **move
only** (affine):

```
var b = Buf[Int].with_capacity(1024)
b.push(1)
let xs = b.freeze()      // moves b, returns an immutable List[Int] in O(1)
b.push(2)                // error XO0510: use of moved value `b`
```

- Assigning, passing, sending on a channel, or capturing in a spawned task moves
  the buffer. The compiler tracks moves only for these types. There is no
  general borrow checker.

### 5.4 Shared mutable state

```
let cache = Mutex[Map[Str, User]].new({})
cache.with(fn(var m) { m.put(key, user) })
let n = cache.with(fn(m) { m.len() })               // read only access
let size = cache.with_read(fn(m) { m.len() })       // shared with other readers
let hits = Atomic[Int].new(0)
let now_hits = hits.add(1)                           // returns the new value
```

`Mutex[T].with` has the signature

```
fn (m: Mutex[T]) with[R, E, fx: Effects](f: fn(var T) -> R ! E uses fx) -> R ! E uses fx
```

- The closure's return value (or error) is the result of `with`.
- Mutations to the `var` parameter are written back when the closure returns
  successfully. If it returns an error or faults, the protected value is left
  unchanged.
- A closure parameter declared without `var` gets a read only view.
- To keep changes and still report failure, return the outcome as the
  closure's success value and apply `?` outside:
  ```
  let outcome = q.state.with(fn(var st) -> Result[Job, QueueErr] {
    st.expire_leases(now)                  // kept
    if !st.token_valid(id, token) { return Err(QueueErr.StaleToken) }
    Ok(st.ack(id))
  })
  outcome?
  ```
- Warning `XO0804` (split critical section): a value read inside one
  `m.with(...)`, or derived from one, is used inside a later `m.with(...)` on
  the same mutex, or a branch on it decides whether the later `with` runs.
  Another task may have changed the state in between (check then act). Do the
  check and the action in one `with`.
- `with` cannot be called re entrantly on the same mutex within one task
  (fault `XO-F004` with both acquisition sites).
- `m.with_read(f)` (std.md 2.7) is shared read only access: `f` gets the
  value read only, readers run together, and `with` excludes them. Writers
  are preferred, so a reader must not wait for a reader that has not
  entered yet. A value read in `with_read` and used in a later `with` gets
  `XO0804` like one read in `with`.

### 5.5 Memory management

Tracing GC, plus request scoped arenas (decision 0066; on by default, off
with `--no-arenas`). An arena is memory owned by one HTTP request served by
`http.server` (std.md 4.4) and reused by a later request once the handler has
returned and the response is written. The compiler and the runtime place in it
only memory that provably cannot outlive the request: the request's header,
parameter, and query tables and its body when the handler uses the request
only through `json`, `param`, `param_int`, `query`, `header`, and its `Str`
fields (and not in a nested closure); the response built by `http.json` or
`http.text` that is directly the handler's result; and the `List` made by
`m.values()`, `m.keys()`, `m.entries()`, or `s.to_list()` when it is only
consumed in its own expression. Everything else, including every value a
program stores, sends, captures, returns, or hands to Go, is ordinary garbage
collected memory, so arenas are invisible: a program prints the same with and
without them. A user written `arena scope` block remains **[P2]**.

---

## 6. Effects and capabilities [P1 checking, P2 replay]

### 6.1 Effect labels

The builtin effects are:

| Effect | Sub effects | Authority |
|---|---|---|
| `net` | `net.dial`, `net.listen` | Sockets, HTTP, DNS |
| `fs` | `fs.read`, `fs.write` | Files and directories |
| `clock` | | Wall clock, timers, sleep |
| `rand` | | Randomness (crypto and math) |
| `env` | | Environment variables, args |
| `proc` | | Spawn processes, signals, exit |
| `stdio` | | Standard input, output, error |
| `ffi` | | Foreign calls (P3) |

A function with no `uses` clause is **pure**: it performs no capability
effects. It may still allocate, fault, be cancelled, log, and use
synchronized state (`Mutex`, `Atomic`, `Chan`).

Logging (`std/log`) is ambient: it needs no capability and is not an effect.
Log output goes to the sink configured in `main` (standard error by default).
It is recorded during record and replay like any other output.

Effect labels live in their own namespace: a parameter or module named `fs` or
`net` does not conflict with the effect `fs` or `net`.

### 6.2 The uses clause, effect variables, and subsumption

```
pub fn sync(src: Fs, out: Stdio) -> () ! SyncErr uses fs.read, stdio { ... }
```

- A function's effects are the union of the effects of everything it calls.
- `pub` functions and interface methods are not inferred: their `uses` clause
  is the declared set, and no clause means pure. Non pub functions and
  closures have their effects inferred.
- Performing an effect outside the declared set is error `XO0701`, which shows
  the call chain that performs it. Declaring more is allowed (for interface
  stability) and noted by `xo vet`.
- **Function types** carry effects. One effect: `fn(Int) -> Int uses clock`.
  Several effects are parenthesized: `fn(Int) -> Int uses (net, clock)`.
- In a declaration clause the parentheses are optional:
  `uses net, clock` and `uses (net, clock)` are the same.
- **Subsumption.** A function value is accepted where a function type is
  expected if its effects are a subset of the expected effects (`fs.read` is a
  subset of `fs`) and its error type converts to the expected one. A function
  that cannot fail is accepted where a failing one is expected. A pure function
  is accepted everywhere a function of the same shape is.
- **Effect variables** let a higher order function pass through its callback's
  effects. Declare them in the type parameter list with the `Effects` kind:
  ```
  pub fn run_pool[T, R, E, fx: Effects](
    jobs: List[T],
    workers: Int,
    f: fn(T) -> R ! E uses fx,
  ) -> Report[R, E] uses fx { ... }

  pub fn retry[T, E, fx: Effects](clock: Clock, f: fn() -> T ! E uses fx) -> T ! E
    uses (clock, fx) { ... }
  ```
  An effect variable is instantiated at each call from the argument's effects,
  and may be empty.
- **Structs may take effect parameters** (`struct Cache[K, V, fx: Effects]`),
  and effect sets may be written as type arguments (`Cache[Int, Item, net.dial]`).
- **Effect arguments may be omitted.** Writing `Cache[Int, Item]` leaves the
  effect arguments to inference. Either all of a type's effect arguments are
  written or all are omitted.
  - In a local binding they are inferred from the value.
  - In parameter and receiver types they become implicit effect variables of
    the function. These count as declared: the function may perform them
    without naming them in `uses`.
  - In result and error types they are inferred from the function body, so a
    constructor `fn new_cache(...) -> Cache[Int, Item]` returns the effects of
    the loader it was given.
  - In a struct field they become implicit effect parameters of the struct.
- A non pub function with a `uses` clause is checked against it. Test blocks
  may perform any effect on their `os`.

### 6.3 Capability values

Effects describe what a function may do. Capability values grant the authority
to do it. The only source of capabilities is `main`:

```
fn main(os: Os) ! {
  let srv = http.server(os.net.listen(":8080")?, router(os.net))
  srv.serve()?
}
```

`Os` has fields `net: Net`, `fs: Fs`, `clock: Clock`, `rand: Rand`, `env: Env`,
`proc: Proc`, `stdio: Stdio`, and `tasks: task.Group` (program lifetime task
group, core 7.1). Each capability type is a predeclared interface,
so test fakes and attenuated versions satisfy it. Their methods are in
`spec/std.md`.

`os.fs` is rooted at the filesystem root and accepts absolute paths
(`/home/me/a`, `C:/data`). Relative paths given to `os.fs` resolve against the
working directory. Capabilities can be attenuated. An attenuated capability
has the same type; operations outside its authority fail with a `Denied`
error. Inside a sub capability, paths are relative to its root:

```
let uploads = os.fs.sub("/var/app/uploads").read_only()
```

A library cannot open a file or a socket unless it is handed a capability. A
dependency's reach is visible in its signatures.

### 6.4 Tests and fakes

See section 9.2: every test block receives `os: TestOs` with fake capabilities.

---

## 7. Concurrency [P1 basic, P2 deterministic]

### 7.1 Scopes and tasks

```
let page = scope s {
  let a = s.spawn(fn() { fetch_user(db, id) })
  let b = s.spawn(fn() { fetch_orders(db, id) })
  render(a.await?, b.await?)
}
```

- A scope is an expression; its value is the body's value.
- `s.spawn(f)` starts a **foreground** task and returns a `Task[T, E]` handle.
  The handle may be discarded.
- `s.spawn_background(f)` starts a **background** task (a daemon such as a
  cache evictor). Background tasks are cancelled automatically when the body
  finishes.
- `t.await` waits for a task and yields `T ! E`.
- **Exit rules.** When the body completes normally, the scope cancels its
  background tasks and waits for all tasks to finish. When the body exits
  early (`return`, `?`, `break`, error, or fault), the scope cancels all tasks
  and waits for them to unwind. No task ever outlives its scope.
- **Child failure.** If a task fails with an error that is not consumed by an
  `await`, or faults, the scope cancels everything else and the error
  propagates out of the scope as if by `?`: it must convert to the enclosing
  function's error type (error `XO0801` otherwise). A task that fails only
  because it was cancelled does not count as a failure.
- `s.cancel()` cancels all tasks of the scope (not the body).
- **Task groups.** `s.group()` returns a `task.Group`, a handle that can be
  stored in a struct or passed to functions so code can start tasks later
  (`g.spawn(f)`, `g.spawn_background(f)`). Tasks in a group belong to the
  scope that created it and never outlive it. A value of a type that contains
  a `Group` cannot be returned from, or stored outside, the creating scope
  (error `XO0802`). `main` receives `os.tasks`, a group that lives for the
  whole program, for services such as caches that refresh in the background.
  Tasks started on a group must not fail with an error the creating scope
  cannot convert (`XO0801`); a failing group task fails that scope. When the
  group's origin is not visible (it came from a field or parameter), a task
  started on it must not fail unless its handle is bound and awaited.
- The escape rule applies to values derived from that scope's `s.group()`.
  `os.tasks` lives for the whole program and may be stored anywhere.
- A task's error is consumed only if its handle is bound with `let` and
  awaited.
- Handles from an enclosing scope may be awaited inside `task.with_timeout`.
- Closures passed to `spawn` capture by value. Move only values are moved.
- The scope tree is the context: deadlines, cancellation, request ids, and
  trace spans flow down implicitly. There is no `ctx` parameter.
- Deadlines are set with `task.with_timeout` (see `spec/std.md`), which runs a
  closure in a child scope and returns `T ! Timed[E]`.

### 7.2 Cancellation

- Cancellation is **sticky**: once a task is cancelled, every cancellation
  point it reaches reports cancellation.
- Blocking operations are cancellation points: effect calls, channel
  operations, `await`, `clock.sleep`, `Mutex.with` and `Mutex.with_read`
  while waiting.
- A blocking operation that already returns a `Result` reports cancellation
  as its `Canceled` error variant (`http.ClientErr.Canceled`,
  `FsErr.Canceled`, ...). Code may map it to its own error, for example
  `FetchErr.Cancelled`, but cannot continue doing blocking work.
- A blocking operation that cannot fail (`clock.sleep`, `ch.send`, `ch.recv`,
  `t.await` in a cancelled task) unwinds the task immediately, running
  `defer` blocks.
- `task.is_canceled() -> Bool` reports the current task's state without
  blocking.
- `defer` blocks run shielded (3.5).

### 7.3 Channels

```
let ch = Chan[Job].new(cap: 64)
ch.send(job)                // blocks when full
let j = ch.recv()           // Job? : None when closed and drained
ch.close()

select {
  j = jobs.recv() => handle(j)
  results.send(r) => count += 1
  after 5s => log.warn("idle")
}
```

- `ch.try_send(x) -> Bool` and `ch.try_recv() -> T?` never block.
- `for x in ch { ... }` receives until `ch` is closed and drained; each receive
  is a cancellation point.
- `after` arms require the `clock` effect.
- Sending moves move only values.

### 7.4 Deterministic scheduling [P2]

Every `xo test` runs its tasks on a deterministic scheduler, seeded with
`--seed=N` (default 1): one task runs at a time, and at each scheduling point
the next task is chosen from the seed, so the same seed always produces the
same interleaving. Scheduling points are channel operations (including
`select`, `try_*`, and `close`), `await`, `Mutex.with`, `Mutex.with_read`,
`Semaphore.with`,
`Once.get`, `spawn`, `Atomic` operations, `sleep`, `task.is_canceled()`,
`task.stop_requested()`, and effect calls. Failing seeds are printed and can
be pinned with `test "x" seed 8812`.

`xo test --explore=N` reruns each concurrent test that passed under N further
seeds and reports the first failing seed. `xo run --seed=N` (or `XO_SEED=N` for
any built binary) uses the same scheduler with the real clock; interleavings
are then reproducible as long as timer and I/O timing does not decide them.

An Xo function that Go calls on a goroutine of its own (`use go`, for
example `sync.WaitGroup.go_`) runs as a scheduled task: Go's goroutine waits
until the scheduler admits the call, which happens only at a scheduling
point after Go's goroutines have settled, in a fixed order (by callback,
then arguments), so the seed decides how callbacks interleave. Once an Xo
function has crossed into Go, the return of every Go call is a scheduling
point. Go code woken by a Go timer or I/O of its own is not reproducible
(decision 0073).

The deterministic scheduler belongs to the Go backend. Native builds
(`--backend=llvm` and `--backend=arm64`, decision 0068) run tasks on the real
scheduler only, on a pool of threads: `xo run --seed` is refused with them and
`XO_SEED` has no effect, and a native program whose output depends on how its
tasks interleave may print differently from run to run. The rest of core 7
means the same on every backend.

### 7.5 Process signals and graceful shutdown

Shutdown has two phases, so programs can finish queued work without fighting
sticky cancellation:

1. **Stop requested.** The first SIGINT or SIGTERM cancels nothing. It sets
   the program's stop state: `task.stop_requested()` returns `true` and the
   channel `task.stopping()` is closed, so loops can `select` on it. Servers
   started with `serve` stop accepting new connections and finish in flight
   requests. Readers stop reading; workers drain what is queued.
2. **Cancel.** When `main`'s body has not returned after the grace period
   (default 30s, set with `os.proc.set_grace(d)`), or on a second signal,
   `main`'s root scope is cancelled as in 7.2: blocking calls unwind and
   `defer` blocks run. A third signal exits immediately with status 130.

A program that ignores phase 1 behaves as before, only 30 seconds later; a
program that wants the old behavior calls `os.proc.set_grace(0s)`.

```
fn reader(os: Os, queue: Chan[Str]) {
  while {
    select {
      line = read_line(os) => queue.send(line)
      _ = task.stopping().recv() => break
    }
  }
  queue.close()                    // workers drain the queue, then exit
}
```

In tests, `os.proc.signal_after(d)` delivers the first signal to a program run
by `testing.run_main`; `os.proc.signal_after(d)` called again delivers the
next one.

Under `testing.run_main`, a third signal makes `run_main` return 130 after
`main` has unwound (the test process itself does not exit), and requests
handled in memory by a fake network observe the stop state but are not
cancelled by it.


---

## 8. Faults [P1]

A **fault** is a bug, not an error: overflow, out of bounds, failed contract,
failed `Int32(x)` conversion, evaluated hole, explicit `fault("msg")`.
`fault` has type `Never`.

- A fault terminates the current task. Its scope treats it like a failed child
  and cancels siblings. A fault reaching `main` exits the process with status
  70 and writes a crash bundle (section 11.2).
- `task.isolate(f) -> T ! Isolated[E]` runs `f` in a child scope and converts a
  fault into a value. `std/http` isolates every request and returns 500.
- Every fault message includes the evaluated values:
  `index 7 out of range for List of length 5 at users.xo:41:18`.
- The native backends unwind a fault the same way (decision 0085: deferred
  code, scopes, Mutexes, and references are released on the way); they
  print the fault without its frames, write no crash bundle, and
  `Fault.frames` of `task.isolate` is empty there.

There is no undefined behavior. Debug and release builds have identical
semantics; release builds may only disable contract checks that are explicitly
marked `requires(debug)` or `ensures(debug)`.

---

## 9. Contracts and tests [P1]

### 9.1 Contracts

```
fn transfer(var from: Account, var to: Account, amt: Money) -> () ! TxErr
  requires amt > 0
  requires from.balance >= amt else TxErr.Insufficient
  ensures from.balance + to.balance == old(from.balance + to.balance)
{ ... }

struct Account { id: Int, balance: Money } invariant balance >= 0
```

- `requires expr` faults on violation. `requires expr else E` returns `Err(E)`
  instead, which makes input validation declarative. A function may consist
  only of contracts and an empty body.
- `ensures` may reference `result` (the success value) and `old(expr)`.
- `invariant` is checked after construction (including from defaults) and
  after every `var` method.
- Contract failures print every sub expression's value:
  ```
  ensures failed: from.balance + to.balance == old(from.balance + to.balance)
    from.balance = 40   to.balance = 50   old(...) = 100
  ```
- `xo test` fuzzes every function with contracts using generated inputs that
  satisfy `requires`, and checks `ensures`. Capability parameters receive
  seeded fakes. Contract clauses must be pure.

### 9.2 Tests

```
test "fetch missing user" {
  let store = new_store()
  expect(store.get(7)) == Err(LoadErr.NotFound(id: 7))
}

test "reverse twice" for xs: List[Int] {
  expect(xs.reverse().reverse()) == xs
}

test "retries after 503" {
  let calls = Atomic[Int].new(0)
  os.net.handle("GET https://api.test/x", fn(_req) {
    if calls.add(1) < 3 { http.empty(503) } else { http.text(200, "ok") }
  })
  let body = get_with_retry(os.net, os.clock, os.rand, "https://api.test/x")?
  expect(body) == b"ok"
  expect(os.clock.sleeps()) == [100ms, 200ms]
}
```

- `test` blocks may live in any file. They are compiled only by `xo test`.
- A test body has type `() ! Error`, so `?` fails the test with the error.
- Inside a test block, and anywhere in a `_test.xo` file for `expect` and
  `testing`, three names are predeclared:
  - `os: TestOs` with fake capabilities (in memory filesystem, virtual clock,
    seeded rand, network that fails unless a handler is registered, captured
    stdio, fake env and proc). Fakes have extra control methods such as
    `os.clock.advance(d)` and `os.fs.fail_next(...)`; see `std/testing` in
    `spec/std.md`. Each fake satisfies the matching capability interface.
  - `expect(x)` followed by any comparison: `expect(a) == b`, `expect(n) <= 3`.
    `expect(cond)` alone asserts a `Bool`.
  - `testing`, the `std/testing` module.
- The virtual clock auto advances: when every task in the test is blocked,
  time jumps to the next timer, so sleeps and timeouts finish instantly.
- `for` on a test makes it a property test over generated values.
- Failures found by fuzzing or property tests are shrunk to a minimal input.
  A shrunk contract fuzz failure is written as a permanent test to
  `regressions_test.xo` in the module itself (so it can call private
  functions), named `regression: <function> (seed N)`, but only if the module
  still type checks with it; otherwise `xo test` says why and the seed
  reproduces the failure. Shrunk property test failures are reported with
  their seed and input.
- Real capabilities require `test "name" uses net { ... }` and
  `xo test --real`; `os` then holds real capabilities for the listed effects.

---

## 10. Toolchain and diagnostics [P1]

### 10.1 Commands

| Command | Purpose |
|---|---|
| `xo build [--targets=all\|os/arch,...] [--no-gc]` | Build static binaries; `--no-gc` on a native backend without a garbage collector (below) |
| `xo run` | Build and run for the host |
| `xo check [--no-gc]` | Parse and type check all targets, no codegen; `--no-gc` adds the native build check (XO1201) |
| `xo test [--seed=N] [--explore=N] [--fuzz=10s]` | Tests, examples, contract fuzzing; deterministic scheduling |
| `xo dump <pid>` | Task tree of a running program (11.4) |
| `xo mcp` | Model Context Protocol server over stdio: `check`, `query`, `explain`, `fmt`, `test`, `build` tools returning compact JSON |
| `xo serve` | Resident checker on a unix socket; `xo check` and `xo query` use it automatically when it answers |
| `xo fmt` | Canonical format, no options |
| `xo fix [--dry-run] [--from=draft-0.N] [--edition=YYYY]` | Apply all machine fixes until none apply; `--from` migrates code written for an older draft (interpolation, prelude variants, `let _ =` discards, scope timeouts, Map index reads) |
| `xo vet` | Advisory checks, never errors: checker warnings plus `XO0207` (`??` precedence trap), `XO0208` (each `ignore_err` with its reason), `XO0703` (declared effect never performed), `XO0901` (`dbg` left in code), `XO1103` (use of Go packages per module) |
| `xo query <kind> ...` | Semantic queries (10.3) |
| `xo edit <op> ...` | Semantic edits (10.4) |
| `xo replay`, `xo why`, `xo dump`, `xo debug` | Debugging (section 11) |
| `xo doc` | Docs from doc comments |
| `xo explain <code>` | Long form explanation of a diagnostic |

Every command accepts `--json`. When stdout is not a terminal, `--json` is the
default for `check`, `build`, `test`, and `query`.

**Builds without a garbage collector.** `xo build --no-gc` (also `xo run`,
`xo test`, and `xo check`) builds on a native backend whose memory is
reference counted instead of collected: arm64 on darwin/arm64 and
linux/arm64 hosts, LLVM otherwise; `--backend=llvm` or `--backend=arm64`
picks one, and `--backend=go` with `--no-gc` is a usage error. After type
checking and before any code generation, a check reports every construct,
std API, or effect the native backends do not support as error `XO1201` at
its site, naming the feature and saying whether support is *planned* (build
without `--no-gc` until then) or *impossible* (code from a `use go` package
needs Go's runtime). A program the check accepts builds; the check is
derived from the native code generator itself, so the two cannot disagree.
`xo check --no-gc` runs the same check without building (programs only: a
module without `main` has nothing to lower). The Go backend's deterministic
scheduler (`--seed` of `xo run`, `--explore`) and recording (`--record`) are
not available on native builds (decision 0083). `xo test --no-gc` builds a
native test binary and runs each test in a process of its own against the
fakes of 9.2 (virtual clock, seeded `Rand`, fake environment, streams, and
network) with the same report as the Go backend; a fault fails only its test.
Property tests and `--real` are not available there yet (XO1201, skipped),
contract fuzzing (9.1) is reported as skipped (decision 0085), and error
traces and fault frames are not recorded natively.

**Hybrid builds.** `xo build --hybrid` (Go backend) compiles the hot leaf
functions of the main module (a loop over `Int`, `Float`, `Bool`, and
`List`s of those or of structs of `Int` and `Float` fields, with no
allocation) to native assembly linked into the Go program; everything
else, and every program the native backends cannot lower, stays Go.
Behavior is the Go backend's, faults included, except that fault frames
omit the assembly functions and `Float` results may differ in the last bit
where the Go backend fuses a multiply and an add (decision 0084).

### 10.2 Diagnostic format

```json
{
  "code": "XO0201",
  "severity": "error",
  "message": "result of `fetch_user` is not handled",
  "span": {"file": "api/users.xo", "line": 18, "col": 3, "end_line": 18, "end_col": 24},
  "notes": [{"message": "`fetch_user` can fail with LoadErr", "span": {"file": "users/fetch.xo", "line": 4, "col": 1}}],
  "fixes": [
    {"title": "propagate with ?", "edits": [{"span": {"file": "api/users.xo", "line": 18, "col": 24, "end_line": 18, "end_col": 24}, "replace": "?"}]}
  ]
}
```

- Codes are stable forever within an edition.
- Codes are grouped: `01xx` lexical and naming, `02xx` errors and results,
  `03xx` match, `04xx` types and generics, `05xx` mutation and moves,
  `06xx` resolution, `07xx` effects, `08xx` concurrency, `09xx` holes and
  contracts, `10xx` platform, `11xx` Go interop, `12xx` native builds
  without a garbage collector (`XO1201`: not available with `--no-gc`,
  planned or impossible; 10.1).
- The compiler reports all errors it can, not just the first, and never
  cascades: a bad expression has type `<error>` and suppresses dependent
  diagnostics.

### 10.3 Queries

```
xo query type api/users.xo:18:7
xo query def http.server
xo query callers users.fetch_user
xo query effects api.get_user          // with the call chain per effect
xo query holes
xo query api github.com/acme/billing   // exported surface of a module
```

### 10.4 Semantic edits [P1: rename, add-field, change-sig]

```
xo edit rename users.fetch_user users.load_user
xo edit add-field User created: Time = Time.UNIX_EPOCH
xo edit change-sig users.load_user "(db: Db, id: UserId) -> User ! LoadErr"
```

Edits update every reference atomically and are rejected if the result does
not type check.

- References are found through the checker's resolved symbols, never by
  text search. An edit of a program that does not check clean is rejected
  before anything is computed.
- `rename` renames a function, method, or constant and every call and use
  as a value. The new name may repeat the target's qualifier.
- `add-field` adds `name: Type` to the struct declaration and
  `name: default` to every struct literal of the type. The default is a
  migration value; it is not kept as a declared field default. Without
  `= default` the edit is rejected when struct literals exist.
- `change-sig` replaces the parameter list, result, and error type (not
  the clauses). Call arguments are matched to the new parameters by name;
  a parameter renamed in place keeps its position, and its uses in the body
  are renamed. New parameters with a default need no argument. A call that
  passes an argument for a removed parameter, or lacks one for a new
  parameter without a default, rejects the edit, listing those call sites.
  Arguments keep their order (and so their evaluation order); arguments
  that no longer match their position become named.
- The result is a JSON summary: `{op, target, ok, written, reason, files,
  edits, diagnostics, sites}`. `diagnostics` holds the errors that rejected
  the edit; `sites` the call sites or literals that blocked it. `--dry-run`
  checks the edit without writing.

### 10.5 Formatting

`xo fmt` produces one canonical layout from the AST. There are no options.
`xo build` fails on unformatted files in CI mode (`--ci`).

---

## 11. Debugging [P1 basics, P2 replay]

### 11.1 Error traces

Each `?` that propagates an error appends a frame:

```
LoadErr.Db(msg: "timeout")
  at users.fetch_user(id=42)          users/fetch.xo:18
  at api.get_profile(req=GET /u/42)   api/profile.xo:7
  in task "handler#913" (scope api.serve)
```

- Frames record function, location, and parameters that implement `Debug`,
  truncated to 64 bytes each. `Secret[T]` prints `<redacted>`.
- Release builds record parameters only with `--error-args`.

### 11.2 Crash bundles

On a fault reaching `main`, an unhandled error from `main`, or an isolated
fault in a server, the runtime writes `<name>-<time>.xocrash` containing: the
error or fault with frames, the task tree, build id, source hashes, target, and
**[P2]** the effect trace for the failing request. Native builds (`--no-gc`)
write none yet (decision 0085).

### 11.3 Record and replay [P2]

All effect calls go through capability values, so the runtime can log each
call and its result, plus scheduler decisions.

- `xo run --record=trace.xotrace` (or `XO_RECORD=trace.xotrace` for any built
  binary) records a full run. Values of type `Secret[T]` are recorded redacted;
  `xo replay` then requires them from the environment and exits with status 3
  naming the missing one. Calls into Go packages (`use go`) are not recorded
  and run again in a replay; an Xo callback that Go calls on a goroutine of
  its own is recorded (where the scheduler admitted it, with its arguments)
  and replayed at the same point, and a replay diverges when Go calls it
  with other arguments (decision 0073).
- In production, a per request ring buffer (`XO_RING=N` requests) is kept and
  attached to crash bundles.
- `xo replay trace.xotrace` (or a `.xocrash`) re executes the program offline
  with identical effect results and interleavings.
- `xo debug --replay trace.xotrace` supports stepping backward.
- `xo why <expr> --at file:line` traces where a value came from (the
  assignments, calls, and effect results that produced it) using the replay.

### 11.4 Live inspection

- `xo dump <pid>` (or SIGQUIT) prints the task tree: each scope, its children,
  and what each task is blocked on, with locations. The program keeps running;
  the dump is also written to `$XO_DUMP_DIR` (or the temp dir) as
  `xo-dump-<pid>.txt`.
- **[P2]** Deadlocks are detected by the runtime and reported as a wait cycle
  with locations (fault `XO-F001`, exit status 70, crash bundle). A task
  counts as blocked when it waits on another task (channel, mutex, semaphore,
  `await`, scope end, `Once`); waits on timers, `after`, `task.stopping()`,
  signals, and I/O do not. In test worlds the report is immediate; real runs
  report after 200 ms with no progress. A task inside a Go call (`use go`)
  counts as blocked only while it waits for its own Go callbacks running on
  goroutines of Go's, those callbacks are blocked in turn, and every
  goroutine's stack shows that nothing else can move (the Go call parked in
  a `sync` wait, no goroutine running, sleeping, or in I/O); the report
  shows the edge as `blocked in Go call sync.(*WaitGroup).Wait at ... for
  callback 3`. A Go call waiting on a Go channel, select, or timer is never
  reported, and nothing is reported while any Go runtime timer is pending
  (a `time.AfterFunc` may release the wait; the check looks again after it
  fires); builds for targets other than darwin and linux never report it
  (decision 0073). Native builds (decision 0068), which
  have no timers, I/O, or signals, report at once when every task is blocked
  and none is queued, with the fault's first line only (no location or list
  of blocked tasks yet).
- `dbg(expr)` prints the expression source and its `Debug` value, returns the
  value, and is rejected by `xo build --ci`.

### 11.5 Debugger protocol

`xo debug --json` speaks a JSON line protocol (breakpoints, conditional
breakpoints, break on contract failure, eval, step, step back with replay).
`xo debug --dap` speaks the Debug Adapter Protocol for editors.

**[P1]** The generated Go code carries `//line file.xo:N` directives, so Delve,
Go stack traces, and pprof report Xo source locations.

### 11.6 Observability

- Every `scope` and every effect call is a trace span. `std/log` is structured
  and attaches the current span and request id automatically.
- OpenTelemetry export is enabled by setting `XO_OTEL_ENDPOINT`.

---

## 12. Targets and platform code [P1]

### 12.1 Targets

`xo build --targets=all` builds every supported target from any host:
`linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64 wasi/wasm32`.

Binaries are static and builds are reproducible: the same source, lockfile,
and toolchain version produce byte identical output. Each build writes an SBOM
(`<bin>.sbom.json`).

### 12.2 Platform specific code

There are no build tags and no filename based platform selection.
`target` is a predeclared compile time constant with fields
`os: TargetOs` (`Linux Darwin Windows Wasi`) and `arch: TargetArch`
(`Amd64 Arm64 Wasm32`):

```
pub fn config_dir(env: Env) -> Str ! Error uses env {
  match target.os {
    Linux => env.get("XDG_CONFIG_HOME") ?? "${env.home()?}/.config"
    Darwin => "${env.home()?}/Library/Application Support"
    Windows => env.get("APPDATA").or_err("APPDATA not set")?
    Wasi => "/config"
  }
}
```

- A `match` on `target.os` or `target.arch` is resolved at compile time; only
  the chosen arm is code generated.
- **Every arm is type checked for every target on every `xo check` and
  `xo build`**, so a broken Windows branch fails on a macOS machine. A missing
  OS is a non exhaustive match error.
- Platform specific APIs live in `std/sys/<os>` and can only be referenced from
  inside the matching arm (error `XO1001` otherwise).

---

## 13. Editions and compatibility [P1]

- `xo.mod` declares an `edition`. The compiler supports every edition ever
  released. Modules on different editions link together.
- A new edition may change syntax and defaults, never the meaning of code in
  older editions.
- `xo fix --edition=YYYY` migrates a module mechanically and must leave it
  compiling. An edition is not released until its rewriter is complete.
- The standard library follows the same rule: `std` APIs are never removed
  within an edition; deprecated APIs ship rewrites in `xo fix`.

---

## 14. Standard library

The prelude (methods on builtin types) and the standard library surface are
specified in `spec/std.md`. Modules:

| Module | Contents |
|---|---|
| `std/http` | Router, server, client, requests, responses, middleware, error to status mapping |
| `std/json` | Derive based encode and decode, `Patch[T]` for partial updates |
| `std/log` | Structured, ambient logging bound to spans |
| `std/task` | `isolate`, `with_timeout`, `is_canceled`, `Semaphore` |
| `std/time` | Time zones and formatting (arithmetic is in the prelude) |
| `std/testing` | Fake capability types, generators, captured logs |
| `std/sql` | Database handles over drivers, in memory driver (draft) |
| `std/crypto` | SHA-256, HMAC, constant time compare, random tokens, password hashing |
| `std/trace`, `std/config`, `std/text`, `std/sys/<os>` | Later drafts |

---

## 15. Grammar summary (informal EBNF)

Every comma separated list allows a trailing comma and may span lines. The
P1 parser (`internal/parser`) is the reference implementation of this grammar.

```
File        = { UseDecl } { TopDecl } .
UseDecl     = "use" ImportPath [ "as" ident ] .
ImportPath  = PathSeg { "/" PathSeg } .               // std/http, github.com/acme/x
TopDecl     = { DocComment } [ "pub" ] ( FnDecl | StructDecl | EnumDecl | IfaceDecl
            | TypeDecl | ConstDecl ) | TestDecl .

FnDecl      = "fn" [ Receiver ] ident [ TypeParams ] Params [ "->" Type ] [ ErrSpec ]
              { FnClause } Block .
FnSig       = "fn" ident [ TypeParams ] Params [ "->" Type ] [ ErrSpec ] { FnClause } .
Receiver    = "(" [ "var" ] ident ":" Type ")" .
TypeParams  = "[" TypeParam { "," TypeParam } "]" .
TypeParam   = ident [ ":" ( Type | "Effects" ) ] .
Params      = "(" [ Param { "," Param } ] ")" .
Param       = [ "var" ] ( ident ":" Type [ "=" Expr ] | "self" ) .   // self: interface methods only
ErrSpec     = "!" [ Type ] .
FnClause    = "uses" ( EffectList | "(" EffectList ")" )
            | "requires" [ "(" "debug" ")" ] Expr [ "else" Expr ]
            | "ensures" [ "(" "debug" ")" ] Expr .
EffectList  = Effect { "," Effect } .
Effect      = ident [ "." ident ] .

Type        = BaseType { "?" } .
BaseType    = QualName [ "[" TypeArg { "," TypeArg } "]" ]
            | "(" ")" | "(" Type ")" | "(" Type "," [ Type { "," Type } ] ")"
            | FnType .
TypeArg     = Type | EffectList .                     // effect sets for fx parameters
FnType      = "fn" "(" [ FnTParam { "," FnTParam } ] ")" [ "->" Type ] [ ErrSpec ]
              [ "uses" ( Effect | "(" EffectList ")" ) ] .
FnTParam    = [ "var" ] Type .

StructDecl  = "struct" ident [ TypeParams ] "{" { Field } "}" [ Invariant ] [ Derive ] .
Field       = { DocComment } [ "pub" ] ident ":" Type [ "=" Expr ] .
Invariant   = "invariant" Expr .
Derive      = "derive" QualName { "," QualName } .
EnumDecl    = "enum" ident [ TypeParams ] "{" { Variant } "}" [ Derive ] .
Variant     = { DocComment } ident [ "(" VField { "," VField } ")" ] .
VField      = ident ":" Type | "from" Type .
IfaceDecl   = "interface" ident [ TypeParams ] "{" { FnSig } "}" .
TypeDecl    = "type" ident [ TypeParams ] ( "=" Type | "struct" "(" Type ")" ) .
ConstDecl   = "const" ident [ ":" Type ] "=" Expr .
TestDecl    = "test" string [ "for" Param { "," Param } ] [ "seed" int ]
              [ "uses" EffectList ] Block .

Stmt        = "let" Pattern [ ":" Type ] "=" Expr
            | "var" ident [ ":" Type ] "=" Expr
            | ConstDecl
            | Place AssignOp Expr
            | "defer" ( Stmt | Block ) | "return" [ Expr ]
            | "break" [ ident ] | "continue" [ ident ]
            | ForStmt | WhileStmt | Expr .
AssignOp    = "=" | "+=" | "-=" | "*=" | "/=" | "%=" .
Place       = ident { "." ident | "." int | "[" Expr "]" } .
ForStmt     = [ ident ":" ] "for" Pattern "in" Expr Block .
WhileStmt   = [ ident ":" ] "while" [ Expr ] Block .

Expr        = IfExpr | MatchExpr | ScopeExpr | SelectExpr | Block | FnLit | Binary
            | "while" Block | "???" .      // infinite loop as an expression, type Never
Binary      = Operand { BinOp Operand } .
Operand     = Unary [ "is" Pattern ] .            // `is` binds like `==`
FnLit       = "fn" "(" [ LParam { "," LParam } ] ")" [ "->" Type ] [ ErrSpec ]
              [ "uses" ( EffectList | "(" EffectList ")" ) ] Block .
LParam      = [ "var" ] ident [ ":" Type ] .
IfExpr      = "if" ( Expr | "let" Pattern "=" Expr ) Block [ "else" ( IfExpr | Block ) ] .
MatchExpr   = "match" Expr "{" { Pattern [ "if" Expr ] "=>" ( Expr | Stmt ) } "}" .
ScopeExpr   = "scope" ident Block .
SelectExpr  = "select" "{" { SelectArm } "}" .
SelectArm   = ( [ ident "=" ] Expr | "after" Expr ) "=>" ( Expr | Stmt ) .
Postfix     = Primary { "." ident | "." int | Call | Index | "?" | "with" StructLit } .
Index       = "[" ( Expr | [ Expr ] ( ".." | "..=" ) [ Expr ] ) "]" .
Call        = [ "[" TypeArg { "," TypeArg } "]" ] "(" [ Arg { "," Arg } ] ")" .
Arg         = [ ident ":" ] Expr .
Literal     = int | float | duration | string | rawstring | bytes | rune | "true" | "false" .

Pattern     = PatAlt { "|" PatAlt } .
PatAlt      = "_" | ident | [ "-" ] Literal [ ( ".." | "..=" ) [ "-" ] Literal ]
            | "(" ")" | "(" Pattern ")" | "(" Pattern "," [ Pattern { "," Pattern } ] ")"
            | [ QualName "." ] ident [ "(" ( ".." | PatFields ) ")" ]   // variant
            | QualName "{" PatFields "}"
            | "[" [ PatElem { "," PatElem } ] "]" .               // ".." at most once
PatElem     = Pattern | ".." .
PatFields   = PatField { "," PatField } [ "," ".." ] .
PatField    = [ ident ":" ] Pattern .
QualName    = [ ident "." ] TypeName .     // optional module prefix
TypeName    = ident .
```

Operator precedence, highest first: postfix (`. () [] ?`), unary (`- ! ~`),
`* / % << >> &`, `+ - | ^`, `.. ..=`, `== != < <= > >= is`, `&&`, `||`, `??`.

Parsing rules the grammar alone does not settle:

- **Type arguments vs. indexing.** `x[...]` holds type arguments when every
  element looks like a type (`UpperCamel`, `mod.UpperCamel`, `(`, or `fn`) and
  either `(` follows or `x` is an `UpperCamel` type name; otherwise it is an
  index. So `json.decode[User](b)` and `Mutex[Int].new(0)` take type
  arguments, while `xs[i]` and `m[Red]` index.
- **`{` in expression position** is an empty map when it is `{}`, a map literal
  when a `:` appears at depth zero before the first newline, and a block
  otherwise. A match arm `=> {}` is therefore an empty map; write `()` for unit.
- **Function types in return position** take a trailing `uses` themselves:
  `-> fn(A) -> B uses clock {` gives the effect to the returned function. Write
  `-> (fn(A) -> B) uses clock {` to put it on the declaration.
- In a function type, a `uses` list with more than one effect must be
  parenthesized, because a following comma would otherwise be ambiguous.
- Only `while { }` (no condition) may appear in expression position.
- Triple quoted strings support escapes and interpolation; a blank first and
  last line are dropped; the common indentation is measured on the source.

---

## Appendix A: complete example

```
// api/main.xo
use std/http
use std/log

pub struct User {
  pub id: Int
  pub name: Str
  pub email: Str?
} derive Json

pub enum ApiErr {
  BadRequest(msg: Str)
  NotFound(id: Int)
}

// http maps handler errors to responses through this interface.
fn (e: ApiErr) status() -> Int {
  match e {
    BadRequest(_) => 400
    NotFound(_) => 404
  }
}

fn (e: ApiErr) display() -> Str {
  match e {
    BadRequest(msg) => msg
    NotFound(id) => "user ${id} not found"
  }
}

struct Store {
  users: Mutex[Map[Int, User]]
}

fn (s: Store) get(id: Int) -> User ! ApiErr
  requires id > 0 else ApiErr.BadRequest(msg: "id must be positive")
{
  s.users.with(fn(m) { m.get(id) }).or_err(ApiErr.NotFound(id: id))?
}

fn routes(store: Store) -> http.Router {
  var r = http.router()
  r.handle("GET /users/{id:Int}", fn(req) -> http.Response ! ApiErr {
    let u = store.get(req.param_int("id"))?
    http.json(200, u)
  })
  r
}

fn main(os: Os) ! {
  let store = Store{users: Mutex.new({1: User{id: 1, name: "Ada", email: None}})}
  let srv = http.server(os.net.listen(":8080")?, routes(store))
  log.info("listening", {"addr": ":8080"})
  srv.serve()?
}

test "missing user is 404" {
  let store = Store{users: Mutex.new({})}
  let resp = routes(store).call(http.new_request("GET", "/users/9"))
  expect(resp.status) == 404
}
```

## Appendix B: open questions

Tracked in `knowledgebase/decisions/` once decided.

1. Method syntax: Go style receivers (current) or `impl` blocks.
2. Whether `Map` iteration order should be insertion ordered (current) or
   sorted, given the cost on the Go backend.
3. Error frame capture cost in release builds.
4. ~~How `http` handler errors map to status codes.~~ Decided in 0019.
5. FFI design and how `ffi` interacts with record/replay.
6. ~~Interpolation syntax.~~ Decided in 0021: `${x}`.
