# Paper test notes: 01-crud-api (spec v0.3)

Files: `main.xo` (types, store, router, main), `users_test.xo` (the five tests).

## 1. Spec gaps

1. **Bare variants of another module's enum in a pattern** (core 2.3 vs 3.5).
   2.3 says "Enums from other modules are qualified with the module"
   (`task.Timed.TimedOut`), but also that the prefix may be omitted "in a
   `match` arm". The pattern table in 3.5 says a unit variant may be "bare or
   qualified". It is unclear whether `Absent`, `Null`, `Value(v)` of
   `json.Patch` may be written bare. Guessed bare (marked `GUESS` in
   `Store.patch`). The alternative would be `json.Patch.Absent`.
2. **`E: Error` with a handler that cannot fail** (std 4.1, core 6.2).
   `Router.handle[E: Error, fx]` requires `E: Error`. A closure like
   `fn(_req) { http.json(200, store.list()) }` has no error type, so `E` is
   presumably `Never`. The spec says a non failing function is accepted where a
   failing one is expected, but never says `Never` implements `Error` /
   `Display`. Guessed it works (marked `GUESS`).
3. **Newline before `uses` / `requires` clauses** (core 1.3 vs 3.2). The
   statement termination rule says a newline after an identifier ends the
   statement, yet every example puts `requires` / `uses` on the next line after
   `-> T ! E` (which ends in an identifier). I followed the examples
   (`fn check_name(name: Str) ! ApiErr` then `requires` on the next line), but
   1.3 does not list this exception the way it lists the leading `.` exception.
4. **Function with only contracts and `! E` but no `-> T`** (core 9.1, 15).
   "A function may consist only of contracts and an empty body" is stated, and
   `fn main(os: Os) !` shows `-> ()` may be omitted before `!`. I combined them
   as `fn check_email(email: Str) ! ApiErr requires ... else ... {}`. Not
   marked as a guess since the grammar allows it, but no example shows it.
5. **Enum prefix in `requires ... else E` and in `or_err(...)` arguments**
   (core 2.3). The list of places where the prefix may be omitted includes
   "an argument", but `or_err[E](e: E)` is generic so there is no expected
   type, and the `else` of `requires` is not in the list. I qualified
   (`ApiErr.NotFound(id: id)`) everywhere to avoid the question.
6. **Tail `if` without `else` as the success value of `() ! E`** (core 2.6,
   3.5). In `Store.delete` the closure body ends with
   `if ... { return Err(...) }`, whose type is `()`. 2.6 says a returned `T`
   means success, so this should be fine, but there is no example of a
   `() ! E` function whose body ends with a statement like this.
7. **`Mutex.with` returning `R ! Never`** (core 5.4, 2.6). `s.db.with(fn(db) {
   db.users.get(id) })` has `E = Never`, so the result type is `User? ! Never`,
   which 2.6 says is the same as `User?`. I chained `.or_err(...)` directly on
   it. Consistent with the Appendix A example, so not marked.
8. **What `{id:Int}` does vs the task's `{id}`**. Not a spec gap, but std 4.1
   says a non integer segment yields 400 "before the handler runs" without
   saying what the body is. I assumed `{"error": ...}`.
9. **"Characters" for the name length** is a task ambiguity: I used
   `rune_count()` (Unicode scalars), not `len()` (bytes) and not grapheme
   clusters, which std does not offer.
10. **Email uniqueness and case** is unspecified by the task; I compare exact
    strings.
11. **`Map[Str, Str]` decode of the error body** (std 5). The 400 body is
    `{"error": "..."}`; I decode it as `Map[Str, Str]` in the test. The JSON
    table says `Map[Str, V]` is an object, so this should work.

## 2. Habits

- Started to write `type Store struct { mu sync.Mutex; users map[int]User }`
  (Go). Changed to a `Store` struct holding a `Mutex[Db]` handle, with all
  state in an immutable `Db` value mutated through `with(fn(var db) ...)`.
- Wanted `if err != nil { return ... }` after decoding the body. Changed to a
  `BadJson(from json.DecodeErr)` variant so `req.json[CreateUser]()?` converts
  automatically.
- Wanted `*string` / `Option<String>` with `#[serde(default)]` for PATCH
  fields. Changed to `json.Patch[Str]`, which also distinguishes `null`
  (clear nickname) from a missing key.
- Started to write `Some(x) => ...` matching on `users.remove(id)` and an
  `Ok(())` at the end of delete (Rust). Changed to `if ... .is_none() { return
  Err(...) }` and let the `()` tail stand as success.
- Reached for `for _, u := range users { if u.Email == email ...}`. Changed to
  `db.users.values().any(fn(u) { u.email == input.email })`.
- Almost wrote `"user {id} not found"` (Rust format style) before remembering
  interpolation is `${id}`.
- Almost wrote a `validate(input) -> Result<(), ApiErr>` with explicit
  `if` checks. Changed to functions made only of `requires ... else` clauses,
  which the spec presents as the declarative way.

## 3. Friction

- **Mutation goes through closures.** Every write has to be a `Mutex.with`
  closure with an explicit `-> User ! ApiErr` return type, and early exits use
  `return` that returns from the closure, not the method. Easy to misread:
  ```
  s.db.with(fn(var db) -> User ! ApiErr {
    if db.users.values().any(fn(u) { u.email == input.email }) {
      return Err(ApiErr.Conflict(email: input.email))
    }
    ...
  })
  ```
  On the plus side, "error leaves the value unchanged" gives free rollback
  for PATCH, which is genuinely nice.
- **Copy, mutate, write back.** In PATCH I copy the user out of the map, edit
  it, and must remember `db.users.put(id, u)`. XO0520 catches forgetting it
  only in some shapes (here the initializer is `.or_err(...)?`, which is not in
  the listed forms `??` or `?` directly after the lookup, so I am not sure the
  check would fire):
  ```
  var u = db.users.get(id).or_err(ApiErr.NotFound(id: id))?
  ...
  db.users.put(id, u)
  ```
- **Three near identical `match` blocks for `Patch` fields.** There is no
  helper like `patch.apply(current)` in std, so each field gets its own
  `Absent => () / Null => ... / Value(v) => ...` match.
- **Enum qualification uncertainty.** Because it is not clear when the prefix
  may be dropped, I wrote `ApiErr.` in front of nearly every construction,
  which is noisy: `.or_err(ApiErr.NotFound(id: id))?`.
- **Validation of PATCH needs the merged value**, so validation happens inside
  the lock instead of before it, unlike create. Not a spec issue, but the
  `requires ... else` style only works on function boundaries, which forced
  the extra `check_name` / `check_email` functions.

## 4. Confidence

- `main.xo`: about 2 to 4 remaining violations, most likely around the two
  `GUESS` spots (bare `json.Patch` variants, `E = Never` in `handle`), the
  clause on the line after `! ApiErr`, and the `()` tail in `Store.delete`.
- `users_test.xo`: about 1 to 2. Likely candidates: the default parameter plus
  named argument in `send`, and a `const` holding a raw string (should be
  fine, but untested ground).
