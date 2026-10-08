# Notes: 08-notes-service in Xo 0.4

Layout (one directory per module, core 1.1 and 4.1):

- `xo.mod`: `module example.com/notes`, `edition 2026`
- `domain/`: `Note`, `Edit`, validation (contracts with `requires ... else`), ids, pure search and cursor pagination, tests
- `store/`: one JSON file per note, temp file plus rename, a `Mutex` around read, compare, write; tests
- `api/`: router and handlers, `ApiErr` implementing `status()` and `display()`; tests
- `server/`: flags, wiring, graceful shutdown via core 7.5; flag tests
- `archtest/`: the dependency rule (pure functions) and the tests that apply it to the real source tree

Checker result: 2 runs, 0 errors at the end. The checker cannot run code, so
none of the tests have been executed.

## Suspected checker bugs

### 1. `testing` is not predeclared outside test blocks in `_test.xo` files

Diagnostic (run 1):

```
api/api_test.xo:6:19: error XO0601: module `testing` not found
store/store_test.xo:3:15: error XO0601: module `testing` not found
```

Code:

```
// store/store_test.xo, no `use std/testing`
fn temp_files(os: testing.TestOs) -> List[Str] {
  os.fs.files().keys().filter(fn(p) { p.contains(TEMP_PREFIX) })
}
```

Spec: core 9.2 says "Inside a test block, and anywhere in a `_test.xo` file
for `expect` and `testing`, three names are predeclared", and std 8 repeats
"`expect` and `testing` are also available anywhere in `_test.xo` files". A
helper function in a `_test.xo` file should therefore resolve
`testing.TestOs` without an import.

Workaround: added `use std/testing` to both test files. The checker accepted
it without an unused or duplicate import complaint, which is itself a bit
odd if `testing` were predeclared there (it would shadow or duplicate a
predeclared name). I kept the import.

### 2. A cascaded diagnostic (minor)

Run 1 also reported `XO0405: cannot infer the type of parameter 'p'` on the
`filter(fn(p) { ... })` above. It was caused by the unresolved `testing`
type and went away once that resolved. Core 10.2 says the compiler "never
cascades: a bad expression has type `<error>` and suppresses dependent
diagnostics". An unresolved parameter type should have suppressed this one.

## Spec gaps and guesses

1. **Where does `xo test` run, and can a test read the real source tree?**
   The dependency rule test must read the project's `.xo` files. Core 9.2 says
   real capabilities need `test "name" uses fs.read { ... }` and `xo test
   --real`, but does not say what the working directory is (the module
   directory, like Go, or the project root) or what `os.fs` is when the test
   is run without `--real` (skipped? fakes?). I look for `xo.mod` in `.` and
   then `..`. If the test runs on fakes without `--real`, it fails with
   `NotFound` instead of being skipped; that is a gap the spec should close.
   I also split the rule into pure functions (`archtest/deps.xo`) with a fake
   free test, so the rule logic is tested either way.
2. **No way to enforce imports at compile time.** There is no
   `internal/`-like visibility or import allow list in `xo.mod`, so the
   dependency rule is a test that parses `use` lines. A per module allow list
   in `xo.mod` would make it a compile error instead.
3. **Ranges as values.** I first wrote `(0..10).map(...)`; the spec only uses
   ranges in `for`, slicing, and patterns, and there is no `Range` type in
   std, so I rewrote these as `for` loops before running the checker.
4. **Tuples and `Ord`.** No statement on whether tuples are `Ord`, so for the
   newest first order I used two stable `sort_by` calls (id, then created
   millis) and `reverse()` instead of `sort_by(fn(n) { (n.created, n.id) })`.
5. **`Time` precision for cursors.** `Time.from_unix` takes whole seconds, so
   a cursor cannot round trip a `Time` exactly. I compare by
   `created.unix_millis()` everywhere, which means two notes in the same
   millisecond are ordered by id. Fine here, but there is no
   `Time.from_unix_millis`.
6. **Opaque cursor encoding.** std has no base64 or hex helper except going
   through `json.encode` of `Bytes`. The cursor is `"<millis>.<id>"`, URL safe
   and validated on decode, documented as opaque, but not actually opaque.
7. **`Mutex` of nothing.** I needed a lock with no data (the data lives on
   disk). I used `Mutex[Int]` counting committed writes rather than guessing
   whether `Mutex[()]` is allowed.
8. **`http.stream` body in `Router.call`.** Unspecified whether the in process
   `call` collects a streamed body into `Response.body`. The export test
   assumes it does.
9. **Graceful shutdown.** Handled by the runtime (core 7.5 plus `serve`
   returning `Ok(())` after draining). There is nothing to write beyond
   logging after `serve()`. Testing it is not possible as specified: a
   `testing.run_main(main, os)` test would block in `serve()` forever, since
   there is no fake signal (`FakeProc` has no `signal` method). I dropped that
   test before the first run.
10. **Fake filesystem paths.** `MemFs.fail_next` paths are relative to the
    test's `os.fs` root, and the store writes through `os.fs.sub("data")`, so
    I used `"data/<id>.json"` and `"data/.tmp-*"`. Whether `create_temp`
    counts as a `Write` op for `fail_next` is not said; the test passes either
    way (if `create_temp` fails, no temp file exists).
11. **If-Match parsing.** The task says a missing header is 428 and a stale
    version is 412. A header that names no version (`If-Match: garbage`) is
    unspecified; I return 412 (it cannot match). `If-Match: *` is not
    supported.
12. **Title length.** "1 to 200 characters" is read as Unicode scalar values
    (`rune_count`), not bytes, and a whitespace only title counts as valid.

## Go and Rust habits caught

- Reached for `(0..n).map(...)` (Rust iterator habit). Rewrote as loops.
- Almost named a local `store` while also importing module `store` (Go
  habit of naming a variable after its package). Used `st`.
- Wanted `defer` cleanup in the Go style "on error" form. In Xo `defer` is
  block scoped and shielded, which fit better: a `committed` flag read when
  the defer runs makes temp cleanup also cover cancellation.
- Almost wrote `let _ = s.dir.remove(tmp)` (Go `_ =`). Core 2.6 makes that
  XO0206; wrote `.ignore_err("reason")`.
- Wrote match arms separated by commas on one line (Rust). Arms are
  newline separated, so short matches became helper functions
  (`after_cursor`).
- Wanted `http.Error`-style early returns writing to a response; Xo handlers
  return `Err(ApiErr)` and the router maps status through `StatusError`,
  which removed a lot of code.

## Friction

1. **`testing` in helper signatures.** Shared test helpers need the
   `TestOs` type in their signature, and the checker did not resolve
   `testing` there (bug 1). Every multi test file wants helpers like:
   ```
   fn new_router(os: testing.TestOs) -> http.Router ! Error {
     let st = store.open(os.fs.sub("data"))?
     routes(st, os.clock, os.rand)
   }
   ```
2. **Error enums across modules need a lot of mapping code.** The `from`
   variant makes `?` easy, but `status()` and `display()` must then match the
   nested enum from another module with fully qualified patterns:
   ```
   Store(inner) => match inner {
     store.StoreErr.NotFound(..) => 404
     store.StoreErr.Conflict(..) => 412
     ...
   }
   ```
   Every new `StoreErr` variant breaks `api` (good for exhaustiveness, but
   noisy). A way for `StoreErr` itself to implement `StatusError` without
   `store` importing `http` would help.
3. **The dependency rule cannot be expressed in the language.** It needs a
   test that reads source text with a real filesystem capability, and the
   spec leaves both the working directory and the without `--real` behavior
   open (gap 1). Compare Go, where `go list -deps` or a test with
   `go/build` is standard.
4. **One line matches are not possible.** Small decisions like the cursor
   check need a multi line `match` or a helper:
   ```
   fn after_cursor(n: Note, after: Cursor?) -> Bool {
     match after {
       Some(c) => { ... }
       None => true
     }
   }
   ```
5. **No range values, no tuple ordering, no hex or base64.** Each forced a
   small workaround (loops, double stable sort, plain text cursor).
6. **Testing graceful shutdown is not possible** with the documented fakes,
   because nothing can deliver a signal or cancel `main` from a test.
