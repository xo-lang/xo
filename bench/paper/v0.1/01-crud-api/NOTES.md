# Paper test notes: 01-crud-api

Files: `store.xo` (model, validation, store), `main.xo` (routes, `main`),
`users_test.xo` (the five required tests).

## 1. Spec gaps

| Spec section | Gap | What I guessed |
|---|---|---|
| 1.6 / Appendix A | `"GET /users/{id:Int}"` is a plain string, so by 1.6 `{id:Int}` would be interpolated. The appendix uses it anyway. | Used raw strings `r"GET /users/{id:Int}"`. Either the appendix is wrong or route strings are special. |
| 2.6 | No syntax for failing early from a `! E` function other than `?` and `requires ... else`. | `return err(ApiErr.Conflict(...))`. |
| 2.6 | Whether passing a failing call straight into a `Result[T, E]` parameter counts as "binding the Result" (XO0201). | Yes: `reply(201, create_from(store, req))`. |
| 2.6 | Whether a discarded non Result value after `?` (`s.create(ada())?` as a statement) is allowed. | Allowed. |
| 2.6 / 3.4 | Whether `ok(v)` / `err(e)` are usable as `match` patterns on a `Result`. | Yes. |
| 2.3 / 3.4 | How a `from` variant binds in a pattern (`BadJson(inner)`), and whether `json.DecodeErr` is `Display`. | Positional binding; it is Display. |
| 2.9 / 2.7 | Whether `Json` (a derive) is also an interface usable as a constraint. | `fn reply[T: Json](...)`. |
| 2.9 | What `derive Display` prints for an enum. | Avoided it; wrote `message()` by hand. |
| 2.9 / 14 | How `derive Json` handles absent keys for `T?` fields. | Absent key decodes to `none`. Also cannot express "key present with null" vs "absent" for PATCH clearing `nickname`, so PATCH can only set a nickname, never clear it. |
| 3.1 | Whether unused function parameters count as unused bindings (XO0110). | Used `fn(_)` for the unused `req`. |
| 3.2 | Whether an empty body `{}` is allowed and has type `()`. | Yes, for contract only functions `check_name` / `check_email`. |
| 5.2 | Whether a rebinding method works on a field of a `var` binding (`st.users.put(k, v)` where `st` is var). Only `xs.push` on a var binding and `u.name = ...` are shown. | Yes, it rebinds `st.users`. |
| 5.4 | Signature of `Mutex.with`: what it returns, and whether a fallible closure makes it return a `Result`. Appendix shows `with(fn(m) {...})` returning a value; 5.4 shows `fn(var m)`. | `with[R](f: fn(var T) -> R) -> R`, so `s.state.with(fn(var st: State) -> User ! ApiErr {...})?` works. Also assumed a non var closure param gets a read only copy. |
| 6.4 / 9.2 | Test blocks have no declared error type. Unclear whether `?` is allowed in a test or how a test fails besides `expect`. | `?` allowed and fails the test; `fault("...")` in a helper fails the test. |
| 9.2 / 14 | `expect(a) == b` is shown but its type and whether it can be a match arm expression are not. | Used it as a statement and as a match arm. |
| 14 (std/http) | No API listed beyond Appendix A (`http.server`, `srv.handle`, `req.param_int`, `http.json`, `srv.serve`). | `req.body() -> Bytes`, `http.Response`, `http.Request`, `http.empty(204)`. Whether status codes come from the error type is Appendix B open question 4; I sidestepped it by returning `http.Response` with no error. |
| 14 (std/json) | No signatures. | `json.decode[T](Bytes) -> T ! json.DecodeErr`. |
| 2.1 / 14 | No `Str`, `Map`, `List`, or `Int` method surface is defined. | `Str.rune_count()`, `Str.contains()`, `Map.get -> V?`, `Map.put`, `Map.remove`, `Map.values() -> List[V]`, `List.sort_by(key)`, `Int.parse(Str) -> Int?`. |
| 2.1 | "1 to 100 characters": bytes or runes? `Str.len()` meaning is not defined. | Runes. |
| 6.3 / 12.2 | `os.env.get` return type. | `Str?`, matching the 12.2 example `env.get(...) ?? ...`. |
| 6.1 | Whether `std/log` needs an effect (it writes somewhere). | Not declared; `main` is non pub so effects are inferred anyway. |
| 2.2 / 9.1 | Whether `invariant` on a struct with field defaults (`State`) is checked on `State{}`. | Yes; trivially holds. |
| 4.1 | Whether `use` is per file or per module (module shares one namespace). | Per file, Go style: `store.xo` and `main.xo` each import `std/json`. |

## 2. Habits

- Started writing `type Store struct { mu sync.Mutex; users map[int]User }` with a
  pointer receiver `func (s *Store)`. Changed to `struct Store { state: Mutex[State] }`
  with value receivers, relying on `Mutex` being a handle type so copies share it.
- Wanted `if err != nil { return ..., err }` after decoding JSON. Changed to a
  `from json.DecodeErr` variant on `ApiErr` and `json.decode[CreateBody](req.body())?`.
- Wanted Rust style `Option<String>` with `Some`/`None`. Changed to `Str?`,
  `some(...)`, `none` (lowercase, predeclared).
- Wanted a Go style `nextID int` counter on the store. Renamed to `next_id`
  (snake_case enforced, XO0101) and moved it inside the mutex guarded `State`.
- Wrote `for _, u := range users` in my head; changed to `for (id, u) in users`.
- Wanted Go's `w.WriteHeader(404); json.NewEncoder(w).Encode(...)` handler shape.
  Changed to closures returning `http.Response` built by `http.json(status, v)`.
- Wanted a Go sentinel `exceptID int = 0` meaning "no user". Changed to
  `except_id: Int?` with `none`, since xo has optionals and no zero values.
- Wanted to write `validate()` with an `if !strings.Contains(...) { return errors.New(...) }`
  ladder. Changed to `requires email.contains("@") else ApiErr.BadRequest(...)`.
- Reached for Rust `impl ApiErr { fn status(&self) }`. Changed to a Go style
  receiver `fn (e: ApiErr) status() -> Int`.
- Wrote `"GET /users/{id:Int}"` copying Appendix A, then realized it interpolates;
  changed to `r"..."`.
- Wanted `u.nickname = nick` (Rust/Go would need wrapping too, but I typed the
  bare form first). Changed to `some(nick)` because there are no implicit conversions.

## 3. Friction

- **Mutation inside the mutex.** The only way to mutate shared state is a `var`
  closure parameter, and I had to guess both that a failing closure flows its
  Result out of `with` and that `st.users.put(...)` on a field rebinds:
  ```
  s.state.with(fn(var st: State) -> User ! ApiErr {
    if email_taken(st.users, body.email, except_id: none) {
      return err(ApiErr.Conflict(email: body.email))
    }
    ...
    st.users.put(u.id, u)
    st.next_id += 1
    u
  })?
  ```
  Uniqueness check, id assignment and insert must happen under one lock, so all
  the logic lives inside a closure with an explicit full type annotation.
- **No early error return syntax.** Every early failure that is not `?` or a
  precondition needs `return err(...)`, which the spec never shows.
- **Contracts as validation are awkward for "optional field" validation.** PATCH
  validates only present fields, so I could not put `requires` on `patch` itself;
  I split out `check_name` / `check_email` with empty bodies just to hang
  `requires ... else` on:
  ```
  fn check_email(email: Str) -> () ! ApiErr
    requires email.contains("@") else ApiErr.BadRequest(msg: "email must contain @")
  {
  }
  ```
- **Error to status mapping is undefined** (Appendix B q4). I wrote `status()`
  and `message()` by hand and a generic `reply[T: Json]` helper, plus a special
  case for DELETE because `()` is not `Json`.
- **Optional fields cannot express "clear".** `PatchBody.nickname: Str?`
  conflates "absent" and "null", so a PATCH cannot remove a nickname.
- **Raw string requirement for routes.** Interpolation in every plain string means
  any literal with `{` needs `r"..."` or `{{`.
- **Testing HTTP without an http test API.** No way to build a fake
  `http.Request` from the spec, so tests check `ApiErr.status()` at the store
  layer instead of going through the routes.

## 4. Confidence (estimated remaining spec violations)

- `store.xo`: 1 to 3. Most risk is in the `Mutex.with` + `return err(...)`
  combination and field level rebinding (`st.users.put`), which are guesses
  rather than violations of stated rules.
- `main.xo`: 1 to 2. Possible XO0201 on passing a failing call as an argument;
  possible issue with `fn(_)`; std/http and std/json signatures are invented.
- `users_test.xo`: 1 to 2. `?` in test blocks and `fault()` as a test failure
  mechanism are unspecified; `expect(...) == ...` as a match arm may not type check.
