# Task 01 CRUD API: paper test notes (spec 0.2)

Files: `store.xo` (types, errors, validation, Mutex backed store), `api.xo`
(routes and `main`), `api_test.xo` (the five required tests).

## 1. Spec gaps

1. **Qualified generic types** (core 15 grammar, `Type = TypeName [...]`).
   TypeName is not defined, so it is unclear whether `json.Patch[Str]` is
   valid. Guessed yes, because `http.Router` and `json.DecodeErr` are used in
   examples.
2. **Infallible handlers and `handle[E: Error]`** (std 4.1, core 6.2). For
   `GET /users` the handler cannot fail. Subsumption says non failing functions
   are accepted where failing ones are expected, but the spec does not say what
   `E` is inferred to, or whether `Never` satisfies `Error`. Guessed it works.
3. **Named functions as values** (core 3.3). Only closure literals are shown as
   arguments. I passed `bad_body` to `map_err`. Guessed allowed.
4. **Implicit conversion to an interface** (core 2.5 vs std 1). Core 2.5 says
   the only implicit conversions are `Some` wrapping and `?` error conversion.
   std 1 says a value passed where an interface is expected converts
   implicitly. That matters for `return Err("create returned ...")` in a
   `! Error` test helper. This reads as a self contradiction; guessed the std
   rule wins.
5. **`var` closure parameters and the lost update check** (core 5.2). XO0520
   exempts "var parameters and receivers". It does not say whether a `var`
   closure parameter (the `fn(var st)` passed to `Mutex.with`) is a parameter
   for this rule. Assumed yes; otherwise every `with` closure whose last use of
   `st` is a mutation (for example in `delete`) would be flagged.
6. **`expect` in test helpers** (core 9.2). `expect` is predeclared only in test
   blocks, so helpers in `_test.xo` files cannot assert. Not a guess, but a gap
   in ergonomics: I made the helper return `! Error` instead.
7. **`derive Json` and non `pub` fields** (std 5). The table says "keys are field
   names" and says nothing about visibility. I left all fields non `pub` since
   the whole program is one module. Not marked as a guess in code.
8. **Which character count** (task + std 2.2). "1 to 100 characters": `len()`
   is bytes, so I used `rune_count()`. Not a spec gap, just a choice.
9. **Function body `{}` vs empty map** (core 1.6 and 3.2). `{}` is "always an
   empty Map" in expression position, while 3.2 says an empty function body
   `{}` has type `()`. I used `{}` as the body of contract only functions
   (`check_name`). Consistent if a function body is a Block, not an expression,
   but the two sentences sit uneasily together.
10. **Request without a Content-Type** (std 4.3). `req.json[T]()` does not say
    whether it checks Content-Type. Tests send none.

## 2. Habits

- Wanted to write a Go style `type Store struct { mu sync.Mutex; users map[int]User }`
  with `s.mu.Lock(); defer s.mu.Unlock()`. Changed to `Mutex[State]` and
  `s.state.with(fn(var st) ...)`.
- Wanted Rust style `Option<String>` and `Ok(u)` for success. Wrote `Str?` and a
  bare `u` as the tail value.
- Wanted to write `if err != nil` style status checks in the test helper and
  then call `expect` (like `t.Fatalf`). `expect` is not available outside test
  blocks, so the helper returns `Err(...)`.
- Started writing a three state patch as `name: Str?` with a separate
  "present" flag (Go `*string` habit). Switched to `json.Patch[Str]` with
  `Absent`, `Null`, `Value(n)`.
- Almost wrote `st.users.remove(id)` and ignored its return like Go's
  `delete(m, k)`. In xo it returns `V?`, which I used as the not found check
  (`.is_none()`).

## 3. Friction

- **Closure return type must be spelled out for `Mutex.with` with errors.** To
  return `Err` or use `?` inside the locked section, I annotated each closure:
  ```
  s.state.with(fn(var st) -> User ! ApiErr {
    check_email_free(st.users, input.email, skip_id: 0)?
    ...
  })
  ```
  Without the annotation it is unclear whether `E` would be inferred from a
  `return Err(...)` deep in a match arm.
- **Three near identical `match` blocks for `Patch` fields.** Each field needs
  `Absent => ()`, `Null => ...`, `Value(n) => {...}`. There is no helper like
  `p.name.apply_to(var u.name)` because a function cannot take a field path as
  an in out argument cleanly (it would need `var` params plus a write back):
  ```
  match p.name {
    Absent => ()
    Null => return Err(BadRequest(msg: "name cannot be null"))
    Value(n) => {
      check_name(n)?
      u.name = n
    }
  }
  ```
- **Validation split between `requires ... else` and in body checks.**
  `requires` is nice for pure field checks, but the email uniqueness check
  needs the map, so it lives in a separate function called inside the lock.
  Validation of a create therefore happens in two places (before and inside
  `with`).
- **Test helpers cannot use `expect`.** `create_ada` has to return
  `User ! Error` and build its own error message.
- **Error to status mapping is implicit.** `ApiErr` must have methods named
  exactly `status` and `display` to satisfy `http.StatusError` structurally.
  A typo (`statuscode`) silently produces 500s; nothing in the code links
  `ApiErr` to `StatusError`.
- **Delete needs a dummy `()` tail.** The `delete` closure ends with an `if`
  without `else`, which relies on "if without else has type ()" to satisfy
  `-> () ! ApiErr`. Works, but reads oddly.

## 4. Confidence

- `store.xo`: about 2 likely violations. Candidates: the `json.Patch[Str]`
  type syntax and enum pattern prefix omission for an imported generic enum;
  the XO0520 rule on `var st` in `delete`.
- `api.xo`: about 1 to 2. Candidates: the infallible `GET /users` handler
  against `E: Error`, and passing `bad_body` by name.
- `api_test.xo`: about 1. Candidate: `return Err("...")` with a `Str` in a
  `! Error` function.
