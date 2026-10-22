# Option and Result for Go programmers

Go returns `(T, error)` and `(T, bool)`; Xo returns one value, `T?` (an
Option: `Some(v)` or `None`) or `T ! E` (a Result: the value or an error
of type `E`). The spec is [core 2.5 and 2.6](../../spec/core.md#25-optionals);
this page maps the Go habits. Every example below compiles with xo
(2026-10-22).

| Go | Xo |
|---|---|
| `v, ok := m[k]; if !ok { v = d }` | `let v = m.get(k) ?? d` |
| `v, ok := m[k]; if !ok { return ... }` | `let v = m.get(k) ?? return Err(E.Missing)` |
| `if v, ok := m[k]; ok { ... }` | `if let Some(v) = m.get(k) { ... }` |
| `if err != nil { return err }` | `let v = load(id)?` |
| `return fmt.Errorf("load %d: %w", id, err)` | a `from` variant: `Load(from LoadErr)`, then `?` wraps it; or `.map_err(fn(e) { ApiErr.Load(e) })?` |
| `fmt.Errorf("port %s: %v", s, err)` into `error` | `let n = parse(s) ?? e => return Err("port ${s}: ${e}")` |
| a `(T, bool)` turned into an error | `opt.or_err(E.Missing)?` |
| `_ = f()` | `f().ignore_err("why this failure does not matter")` |
| `v, _ := f()` (keep the value, drop the error) | `f().ok() ?? d` |
| only the error | `f().err()` (an `E?`) |
| an optional field, `Nick *string` | `nick: Str?`, read with `u.nick ?? u.name` |
| `errors.Is(err, ErrNotFound)` | `match r { Err(LoadErr.NotFound(..)) => ... }` |
| `var pe *ParseErr; errors.As(err, &pe)` | `Err(LoadErr.BadTimeout(inner)) => ...` in the same `match` |
| `if v == nil { panic("...") }` | `opt.or_fault("why it cannot be missing")` |

`??` takes an Option on the left. For a Result, name the error,
`r ?? e => ...` (or `_ => ...`), so an error is never dropped without a
word; `r.ok() ?? d` drops it on purpose.

## Examples

```
enum LoadErr {
  NotFound(id: Int)
  BadTimeout(from DurationErr)
} derive Display

struct User {
  name: Str
  nick: Str? = None
}

fn load(users: Map[Int, User], id: Int) -> User ! LoadErr {
  users.get(id).or_err(LoadErr.NotFound(id: id))
}

fn greet(users: Map[Int, User], id: Int) -> Str ! LoadErr {
  let u = load(users, id)?
  "hello, ${u.nick ?? u.name}"
}

fn timeout(s: Str) -> Duration ! LoadErr {
  Duration.parse(s)? // DurationErr wraps into LoadErr.BadTimeout
}

fn port(s: Str) -> Int ! Error {
  Int.parse(s).or_err("bad port ${s}")?
}

fn describe(users: Map[Int, User], id: Int) -> Str {
  match load(users, id) {
    Ok(u) => u.name
    Err(LoadErr.NotFound(id: n)) => "no user ${n}"
    Err(LoadErr.BadTimeout(inner)) => "bad timeout: ${inner}"
  }
}
```

## The first errors

- **`XO0204`**: `?` on an Option in a function that returns a Result.
  There is no error to return for `None`: say which, `opt.or_err(e)?`
  (the diagnostic offers the fix). Directly in a test block, `opt?`
  fails the test instead (decision 0347).
- **`XO0205`**: `?` in a function that cannot fail (no `!`). Declare the
  error type, or handle the value with `??`, `if let`, or `match`.
- **`XO0601` on `.or_err`**: `.or_err` turns an Option into a Result; on
  a value that is already a Result, replace its error with
  `.map_err(fn(_) { e })` (offered as a fix).
- **`XO0601` on `.or(x)`, `.unwrap_or(x)`, `.or_else(f)`,
  `.get_or_else(x)`, `.value_or(x)`**: Xo writes `opt ?? x` (offered as
  a fix).
- **`XO0406`**: `??` with a Result on the left. Write `r ?? e => d`, or
  `r.ok() ?? d` to drop the error on purpose.

`xo explain CODE` gives each code's long form, and `xo doc Option` and
`xo doc Result` list their methods.
