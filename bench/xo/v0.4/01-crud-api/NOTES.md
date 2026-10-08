# Notes: 01-crud-api (Xo 0.4, with checker)

Layout: `xo.mod`, `store.xo` (User, ApiErr, Store over `Mutex[State]`,
validation), `api.xo` (request bodies, router), `main.xo` (PORT handling,
`main`, one port test), `api_test.xo` (the five required tests plus a list
ordering test and a concurrent create test).

Result: the checker reported 0 errors and 0 warnings on the first run.

## Spec gaps and guesses

1. **xo.mod contents.** Core 4.2 shows an example but does not say whether the
   file is required for a single module or what the module path must look like.
   I wrote `module example.com/crudapi` and `edition 2026`. The checker accepted
   it (it reported module `example.com/crudapi`).
2. **PATCH semantics for `null`.** The task says "any subset of name, email,
   nickname". I used `json.Patch[Str]` for all three: `null` for `nickname`
   clears it, `null` for `name` or `email` is a 400. The spec defines
   `Patch` decoding but not how APIs should treat `null` on a required field,
   so this is a design guess.
3. **Email uniqueness case.** The task does not say whether `A@x.io` and
   `a@x.io` collide. I compare with `to_lower()`.
4. **"1 to 100 characters".** I read "characters" as runes (`rune_count()`),
   not bytes (`len()`), and do not trim whitespace.
5. **Validators as contracts.** I wrote validation as
   `fn validate_name(name: Str) -> () ! ApiErr requires ... else ApiErr.BadRequest(...) {}`.
   Core 9.1 allows a body of only contracts. It is not stated whether
   `requires ... else` contracts are also checked by `xo test` contract
   fuzzing in a way that treats the `else` path as expected (it should, since
   it is a normal error return), so I am guessing that this style is idiomatic
   and not a fuzzing hazard.
6. **`R ! Never` from `Mutex.with`.** `s.state.with(fn(st) { st.users.get(id) })`
   relies on E being inferred as `Never` and `T ! Never` being plain `T`
   (core 2.6), so `.or_err(...)` can be called directly on the `User?`.
   Accepted by the checker; the spec only implies it.
7. **`if` with no `else` as the tail of a `-> () ! E` closure** (`Store.delete`).
   Core 3.5 says `if` without `else` has type `()`, so the closure returns
   success when the branch is not taken. Accepted.
8. **Postfix `with` inside `expect(...) == ...`.**
   `expect(patched) == created with {nickname: "Countess"}` relies on `with`
   being postfix (grammar `Postfix`) so it binds tighter than `==`, and on
   implicit `Some` wrapping in a struct update field. Accepted.
9. **`const` in a `_test.xo` file.** Core 1.1 says those files contain "only
   tests and test helpers". I treated a top level `const ADA = r#"..."#` as a
   helper. Accepted.
10. **Default field values on a non Json struct** (`NewUser.nickname: Str? = None`)
    so the concurrent test can omit it. Allowed by core 2.2.

## Suspected checker bugs

None found. Every construct I was unsure about (list above) was accepted, and
in each case I believe the spec allows it. I did not see the checker accept
anything I believe the spec forbids, but note that I only wrote code I
believed was valid, so this run is weak evidence about false negatives.
Concrete next experiment to probe that side: add a deliberately broken copy,
for example `var u = st.users.get(id).or_err(ApiErr.NotFound(id: id))?` with
the `st.users.put(id, u)` line removed, and confirm `xo-0.4 check` reports
XO0520.

## Go or Rust habits caught

- **Go `*Store` with `sync.Mutex` inside.** I reached for a pointer receiver
  and a mutex field guarding a map. Xo has no pointers; the store is a value
  holding a `Mutex[State]` handle, and all access goes through
  `state.with(fn(var st) { ... })`.
- **Go `strconv.Atoi(r.PathValue("id"))` plus a 400 branch.** Replaced with the
  `{id:Int}` route segment and `req.param_int("id")`, which rejects bad ids
  before the handler runs.
- **Go `*string` / `omitempty` for PATCH bodies.** Replaced with
  `json.Patch[Str]`, which distinguishes absent from `null`.
- **Rust `Ok(())` at the end of a unit returning fallible function.** In Xo the
  success value is just `()` (or an `if` statement of type `()`), and `Ok(x)`
  is only required when T is itself a `Result`.
- **Go `if err != nil { return }` after validation.** Became `validate_name(x)?`
  on a function whose validation lives in `requires ... else`.
- **Rust range habit.** I first wanted `(0..50).to_list()` for the
  concurrent test; there is no documented conversion from a range to a `List`.

## Friction

1. **No range to List conversion.** `task.map_concurrent` takes a `List`, and
   the std spec has no `List.range` or `(a..b).to_list()`. I had to build the
   input with a mutable loop:
   ```
   var inputs: List[Int] = []
   for i in 0..50 {
     inputs.push(i)
   }
   let results = task.map_concurrent(inputs, 8, fn(i) { ... })
   ```
2. **Error semantics inside `Mutex.with` need careful thought.** Because an
   `Err` from the closure discards all mutations (core 5.4), I had to check
   that this is what I wanted in `create` and `update` (it is: validation
   failures must not bump `next_id` or store a half patched user). The closure
   also needs an explicit `-> User ! ApiErr` annotation to use `return Err(...)`
   inside it. For a case where the change must be kept on failure, the
   documented "return a Result as the success value then `?` outside" pattern
   is easy to get wrong.
   ```
   s.state.with(fn(var st) -> User ! ApiErr {
     if st.email_taken(input.email, 0) {
       return Err(ApiErr.Conflict(email: input.email))
     }
     ...
   })
   ```
3. **PATCH merging is verbose.** Each optional field needs a three arm match
   with statement arms, including a `return Err(...)` arm for fields that may
   not be null:
   ```
   match p.name {
     Absent => ()
     Null => return Err(ApiErr.BadRequest(msg: "name cannot be null"))
     Value(v) => u.name = v
   }
   ```
   A helper such as `Patch.apply_to(current: T) -> T?` (or `or(current)`) in
   std/json would cut this to one line per field.

Smaller items:

- `json.Patch` variants are written bare (`Absent`, `Null`, `Value(v)`) in a
  match; I had to infer from core 2.3 that the prefix may be omitted when the
  scrutinee type is known, since std 8.4 only states that rule for testing
  enums.
- The test helper `error_of` needs `! Error` and `.or_err("...")?` with a
  `Str` error, relying on `Str` implementing `Display` to convert to `Error`.
  That works but is not shown anywhere in the spec.
- The checker cannot run the tests, so the actual JSON encoding of
  `nickname: None` (`null`) and the error body shape remain unverified.
