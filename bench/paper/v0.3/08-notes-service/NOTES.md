# Task 08 paper test notes (xo draft 0.3)

Layout:

- `xo.mod` (module `example.com/notes`, edition 2026)
- `domain/note.xo`, `domain/validate.xo`, `domain/search.xo`, `domain/domain_test.xo`
- `store/store.xo`, `store/store_test.xo`
- `api/api.xo`, `api/api_test.xo`
- `server/main.xo` (executable plus flag tests)
- `archtest/deps_test.xo` (dependency rule test)

## 1. Spec gaps

1. **Importing a module of the same project** (core 4.1, 4.2). The examples
   only show std and external modules. I guessed
   `use example.com/notes/domain` (module path from `xo.mod` plus directory).
   Marked GUESS in `store/store.xo`.
2. **What makes a module an executable** (core 1.1, 6.3). `fn main(os: Os)`
   is shown but nothing says which module builds to a binary or what the
   binary is named. Guessed: any module with `fn main`, named after the
   directory. Marked GUESS in `server/main.xo`.
3. **Working directory and `--real` during `xo test`** (core 9.2). The
   dependency rule test needs the real filesystem. The spec says a test with
   `uses fs.read` gets real capabilities under `xo test --real`, but not what
   happens to that test in a plain `xo test` run (skipped? failed? run with
   fakes?), nor what the working directory is. Guessed: the module directory,
   as in Go, so the project root is `..`. Marked GUESS in `archtest/deps_test.xo`.
4. **`fail_next` paths and sub capabilities** (std 8.3, core 6.3). `fail_next`
   paths are "relative to the test's `os.fs` root", but the store works on
   `os.fs.sub("data")` and sees paths like `<id>.json`. I guessed the fake
   matches on the root relative path `data/<id>.json`. Marked GUESS.
5. **Naming std/testing types in tests** (std 8, core 9.2). `FsOp`,
   `TestOs`, `MemFs` are declared under std/testing. It is not said whether
   they are predeclared in tests or must be written `testing.FsOp`. I wrote
   `testing.FsOp.Rename` and avoided naming `TestOs` at all (helpers take
   `Fs`, `Rand`, `Clock` instead). Marked GUESS.
6. **No streaming HTTP response** (std 4.3). `Response.body` is `Bytes`, so
   `GET /export` cannot stream JSON lines; it builds the whole body with
   `StrBuilder`. Marked GUESS in `api/api.xo`.
7. **`Str.lines()` and a trailing newline** (std 2.2). Unclear whether
   `"a\nb\n".lines()` yields a trailing `""`. The export test filters empty
   lines. Marked GUESS.
8. **JSON `Time` precision** (std 5). "RFC 3339 string" does not say whether
   fractional seconds are kept. If they are dropped, a note returned by
   `create` (in memory) and the same note read back (from JSON) have
   different `created` values and different cursor keys. My code always pages
   over notes read back from disk, and the tests use whole second fake times,
   so it works either way, but the spec should say. Not marked (no guess in
   code, the design avoids it).
9. **`invariant` together with `derive`** (core 2.9, 9.1, 15). The grammar
   says `"}" [ Invariant ] [ Derive ]` but `Invariant` has no production. I
   wrote `} invariant version >= 1 derive Json`, which relies on `derive`
   ending the invariant expression.
10. **Does `derive Json` need `use std/json`?** (core 2.9, Appendix A,
    core 3.1). Appendix A imports `std/json` without ever naming `json`,
    which XO0110 (unused import) should reject. Either the import is
    required for the derive (then XO0110 has an exception) or the appendix
    is wrong. I did not import `std/json` in `domain`.
11. **A module made only of `_test.xo` files** (core 1.1). `archtest` has no
    non test file. Not said whether that is a valid module.
12. **Closure error type inference** (core 3.3, 5.4). With
    `Mutex.with[R, E, fx]` nothing fixes `E`, so it is unclear what error type
    a closure using `?` gets. I annotated every such closure
    (`fn(var count: Int) -> () ! StoreErr { ... }`).
13. **Parameters vs. body block for shadowing** (core 3.1). Not said whether
    a `let` in the body that reuses a parameter name is "same block"
    shadowing. I avoided it (`let clean = validate(input)?`).
14. **`while { }` with `break`** (core 3.2). Only the no `break` case is typed
    (`Never`). I assumed `()` with a `break`.
15. **Request URL with a query string** (std 4.3). `new_request(method, url)`
    takes a `url: Str`; I assumed `"/notes?limit=10"` is split into `path`
    and `url.query` so `req.query("limit")` works.
16. **Percent encoded `/` in a `{name}` segment** (std 4.1). Not said whether
    `/notes/..%2F..%2Fetc` matches `{id}` (then the store rejects it, 404) or
    not at all (also 404). The test only checks 404.
17. **No file locking or exclusive create** (std 3.1). Optimistic concurrency
    is enforced by a `Mutex` inside one process. Two processes on the same
    data directory could still lose an update; std has no `O_EXCL` style
    create or lock file primitive to fix that.
18. **`create_temp` return value** (std 3.1). "returns new file path": relative
    to the capability root or to `dir`? I pass `"."` so both readings agree.

## 2. Habits

- Started to write Go `defer os.Remove(tmp)` right after creating the temp
  file. In xo `defer` always runs at block exit, and I only want cleanup on
  failure, so I bound the `Result` and wrote `if res.is_err() { let _ = s.fs.remove(tmp) }`
  followed by `res?`.
- Reached for `sync.Mutex` with `Lock()` / `defer Unlock()` around the
  read, compare, write. Changed to `s.writes.with(fn(var count: Int) -> ... { ... })`,
  which forced the critical section into a closure.
- Wanted `hex.EncodeToString(randBytes)` for ids. No hex in std, so
  `new_id` builds 32 chars with `r.int(0, 16)` and `HEX[i..i + 1]` into a
  `StrBuilder`.
- Wanted a regex (`^[a-z0-9-]{1,32}$`) for tags. Changed to
  `t.runes().all(fn(r) { match r { 'a'..='z' | '0'..='9' | '-' => true ... } })`.
- Wanted Rust `impl fmt::Display for StoreErr`. Changed to the receiver method
  `fn (e: StoreErr) display() -> Str`.
- Wrote `e.display()` from `api` on a `store.StoreErr` at first. Non pub
  methods cannot be called by name across modules (core 2.4), so it became
  `"${inner}"`.
- Wanted Go's `flag` package. Wrote `parse_flags` by hand with a `while`
  loop and `split_flag`.
- Wanted `sort.Slice(notes, func(i, j int) bool {...})` with a two field
  comparison. Changed to a `Cursor` struct with `derive Ord` (field order is
  the comparison order) plus `sort_by(...).reverse()`.
- Wanted Rust `.unwrap_or(20)` on the limit. Used `match` with a `?` in one
  arm, since `??` cannot express "parse failure is a 400".
- Started writing a Go style `signal.NotifyContext` for graceful shutdown.
  Removed it: core 7.5 and std 4.4 make `serve()` return `Ok` on SIGINT.
- Started to type `nil` as the default for optional `Query` fields; wrote
  `= None`.
- Almost escaped the braces in `"[a-z0-9-]{1,32}"` out of habit from the
  0.2 interpolation; in 0.3 braces are literal, so no escape.
- Wanted `go list -deps` / `go/parser` for the dependency rule test. Wrote a
  text scan of leading `use` lines instead.

## 3. Friction

1. **No streaming response.** The task says "streams"; std/http only has a
   `Bytes` body, so export materializes everything:
   ```
   var b = StrBuilder.new()
   for n in domain.newest_first(st.all()?) {
     b.write(json.encode_str(n))
     b.write("\n")
   }
   http.text(200, b.freeze()).with_header("Content-Type", "application/x-ndjson")
   ```
2. **A mutex needs a value even when it only guards effects.** The lock
   exists to serialize filesystem writes, but `Mutex[T]` must protect some
   value, so it holds a write counter that nothing reads, and every closure
   has to increment it or name an unused parameter:
   ```
   s.writes.with(fn(var count: Int) -> domain.Note ! StoreErr {
     let cur = s.load(id)?
     ...
     count += 1
     next
   })
   ```
3. **Every lock closure needs a full type annotation** because `E` in
   `Mutex.with` is otherwise unconstrained (see gap 12).
4. **Cross module error mapping is verbose.** Mapping a store error to an HTTP
   status needs a `from` variant plus nested qualified patterns:
   ```
   Storage(store.StoreErr.Conflict(current: c, ..)) => "version conflict, current version is ${c}"
   Storage(store.StoreErr.Corrupt(..)) | Storage(store.StoreErr.Io(_)) => "internal error"
   ```
5. **Fault injection needs an exact path.** `fail_next` matches a path
   exactly, but the temp file name is random, so a failed temp write during
   `create` cannot be tested; only a failed rename on a known id can:
   ```
   os.fs.fail_next(testing.FsOp.Rename, target, FsErr.Io(path: target, msg: "disk full"))
   ```
   A prefix or glob match (or `fail_next_any(op)`) would cover it.
6. **The dependency rule test is a text scan.** There is no way from a test
   to ask the toolchain (`xo query`) for a module's imports, and the real
   filesystem plus an unknown working directory make the test fragile:
   ```
   test "domain, store and api respect the dependency rules" uses fs.read {
     expect(violations(os.fs, "..")?) == []
   }
   ```
7. **"Clean up only on failure"** has no direct construct. `defer` is
   unconditional, so the pattern is bind, check, re propagate:
   ```
   let res = write_and_rename(s.fs, tmp, file_name(n.id), json.encode(n))
   if res.is_err() {
     let _ = s.fs.remove(tmp)
   }
   res?
   ```
8. **No hex or base64 helpers** for ids and opaque cursors, so the cursor is
   only opaque by convention (`"<millis>-<id>"`).

## 4. Confidence

Estimated remaining spec violations per file (my guess, not counting the
places marked GUESS, which are spec gaps rather than violations):

| File | Estimate | Main risks |
|---|---|---|
| `domain/note.xo` | 0 to 1 | `invariant ... derive` ordering; `ensures result.id == n.id` on a `Note ! ValidationErr` function |
| `domain/validate.xo` | 0 to 1 | `return Err(TitleEmpty)` relying on prefix omission inside `Err(...)` |
| `domain/search.xo` | 0 to 1 | `<` on a `derive Ord` struct; `?` on `Option` with the tail value implicitly wrapped |
| `domain/domain_test.xo` | 1 | `while { }` with `break`; assigning `parse_cursor(text)` inside a match arm |
| `store/store.xo` | 1 | closure annotations in `Mutex.with`; inferred effects of non pub methods called from closures; `res?` as a tail |
| `store/store_test.xo` | 1 to 2 | `testing.FsOp` qualification; `fail_next` path semantics |
| `api/api.xo` | 1 | nested qualified patterns in `status()`; `match` arm ending in `?` that feeds an implicit `Some` |
| `api/api_test.xo` | 1 to 2 | `a.await.status` on a `Never` error task; `expect` inside helper functions; query parsing in `new_request` |
| `server/main.xo` | 0 to 1 | match arms that assign to fields of a `var` struct; tuple with `Str?` element |
| `archtest/deps_test.xo` | 1 to 2 | const map of lists; real `uses fs.read` test semantics; module made only of test files |
