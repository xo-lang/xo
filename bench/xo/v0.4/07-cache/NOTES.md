# Notes: 07-cache (Xo 0.4, with checker)

Layout: one module `example.com/cache` at the folder root.

- `cache.xo`: generic `Cache[K, V, E, fx]`, `Config`, `Metrics`, `CacheErr[E]`.
- `service.xo`: `Item`, `UpstreamErr`, `fetch_item`, `http_fetcher`, `ApiErr`, `routes`.
- `main.xo`: wiring with `os.tasks` for background refreshes.
- `cache_test.xo`, `service_test.xo`: every test the task lists, plus service tests.

Result: 0 errors and 0 warnings on the first check run.

## Design summary

- State lives in one `Mutex[State]`. `get` takes the lock once and gets back a
  `Plan` (`Hit`, `NegativeHit`, `Stale`, `Wait`, `Lead`), then acts outside the lock.
- Single flight: a `Flight` holds `done: Chan[()]` (closed when finished) and
  `outcome: Mutex[Result[V, CacheErr[E]]?]`. The leader runs the load, stores it,
  writes `outcome`, and a `defer` removes the flight and closes `done` (so
  waiters are released even if the leader is cancelled; they then get
  `CacheErr.Canceled`).
- Flights have ids. `invalidate` removes both the entry and the flight, so the
  finishing leader sees it no longer owns the key and does not store. Waiters
  hold the `Flight` handle and still receive the result.
- Timeout and panic: `task.with_timeout(clock, load_timeout, fn() { task.isolate(fn() { loader(key) }) })`,
  mapped to `Timeout`, `Panicked(msg)`, or `Load(e)`.
- Stale while revalidate: the stale hit creates the flight under the lock (so
  only one refresh per key), then `group.spawn_background` runs it.
- LRU: `Map` is insertion ordered, so a hit does `remove` then `put`, and
  eviction removes `keys().first()`.

## Spec gaps and guesses

1. **Type parameters of a generic receiver.** The spec shows
   `fn (xs: List[T]) len()` only for std types and never says how a user method
   on `Cache[K, V, E, fx]` binds `K, V, E`. I guessed Go style (the receiver
   introduces them): `pub fn (c: Cache[K, V, E]) get(key: K) -> V ! CacheErr[E] uses clock`.
   The checker accepted it.
2. **Do constraints repeat?** `CacheErr[E: Display]` needs `Display` for its
   hand written `display`. I put `E: Display` on every type that mentions
   `CacheErr[E]` (`Flight`, `State`, `Plan`, `Cache`, `new_cache`) but not on
   receivers. The spec does not say whether that is required, or whether
   receivers inherit it.
3. **Map key constraint.** Nothing says `K` in `Map[K, V]` must be `Hash + Eq`, or
   whether a generic `K` needs a written constraint. I wrote none; it was accepted.
4. **Effect arguments omitted on receivers.** Core 6.2 says omitted effect
   arguments on a parameter become implicit effect variables. I assumed this
   also applies to a receiver (`Cache[K, V, E]` with no `fx`), and that `uses clock`
   on a `pub` method lists only the extra effect. Accepted.
5. **Match arm joining with failure values.** Core 2.10 says arms must already
   agree. Is an arm of type `V` compatible with an arm `Err(e)` in a function
   returning `V ! E`? Not stated. I avoided it: `get` makes every arm a `Result`
   (`Hit(v) => Ok(v)`), and `load_guarded` uses `=> return Err(...)` (type `Never`).
6. **FakeClock and newly spawned tasks.** std 8.2 says `advance(d)` "lets the
   woken tasks run", but not whether a task that was just spawned (and has not
   reached its first `sleep`) runs before timers fire. For the stale refresh
   test I block the test with `os.clock.sleep(1s)` so auto advance applies.
7. **Faking a 200ms upstream.** `FakeNet.handle` takes a pure
   `fn(http.Request) -> http.Response`, so a fake handler cannot sleep on the
   virtual clock. The 200ms latency is simulated in a loader closure instead,
   and a separate test uses `http_fetcher` through `os.net` for the status mapping.
8. **`req.param` inside FakeNet handlers.** Not stated that FakeNet patterns
   bind `{id}` for `param`. I assumed they do.
9. **Calling a function typed field.** `c.loader(key)` could read as a method
   call. Not covered by the spec; I bound `let loader = c.loader` first.
10. **No range to list.** There is no `(0..100).to_list()`; a small `ids(n)` helper
    builds the list with `push`.
11. **What is negatively cached.** The task says loader errors. I cache only
    `Load(e)`, not `Timeout` or `Panicked`, and a failed background refresh keeps
    the stale value instead of replacing it with the error. These are choices,
    not spec questions.
12. **Function body type with no `->`.** Unclear whether a body with no return
    type may end in a non unit expression (for example `st.flights.remove(key)`,
    which returns `V?`). I wrote `let _ = ...` to be safe.

## Suspected checker bugs

None confirmed. Things to verify with `xo test` once it exists:

- The checker reports no file or declaration counts, so I cannot tell whether
  `test` blocks in `_test.xo` were checked. Core 9.2 says tests are compiled
  only by `xo test`; if `xo check` skips them, a broken test would only show up later.
- It accepted every guess listed above without comment. If any of them
  (implicit receiver type params, constraints not repeated) is meant to be an
  error, the checker is too permissive. Nothing I can point to in the spec
  forbids them, so I am not filing them as bugs.

## Go and Rust habits caught

- Go `singleflight.Group` and `sync.Cond`, Rust `tokio::sync::Notify` or
  `OnceCell`: none exist. I built the broadcast from a closed `Chan[()]` plus a
  `Mutex[Result?]`.
- Go `container/list` plus map for LRU, or mutating `*entry` in place: no
  pointers. Used the insertion ordered `Map` with `remove` then `put`.
- Go `ctx context.Context` for load timeouts: replaced by `task.with_timeout`.
- Go `recover()` / Rust `catch_unwind`: replaced by `task.isolate`.
- Rust `let _ = fallible()`: not allowed for `Result` (XO0206). The background
  refresh uses `.ignore_err("...")`.
- Go `go func() { ... }()` for refresh: needed a `task.Group` handle stored in
  the struct (`os.tasks` from `main` and tests).

## Friction

1. **No one-shot broadcast primitive.** Single flight needs "many waiters, one
   result". The pieces exist but have to be assembled by hand, including
   cancellation cleanup:
   ```
   struct Flight[V, E: Display] {
     id: Int
     done: Chan[()]
     outcome: Mutex[Result[V, CacheErr[E]]?]
   }
   fn (f: Flight[V, E]) wait() -> V ! CacheErr[E] {
     f.done.recv()
     let outcome = f.outcome.with(fn(o) { o })
     outcome ?? Err(CacheErr.Canceled)
   }
   ```
   A `task.Once[T]` or a shareable `Task` handle (several awaiters of one task)
   would remove most of this.
2. **Generic constraint and effect plumbing.** Because the error enum displays
   its inner error, `E: Display` spreads to every generic type that mentions it,
   and the loader's effects need an explicit `fx: Effects` parameter on the
   struct. Uncertain rules (gaps 1, 2, 4) meant guessing each signature:
   ```
   pub struct Cache[K, V, E: Display, fx: Effects] {
     loader: fn(K) -> V ! E uses fx
     ...
   }
   pub fn (c: Cache[K, V, E]) get(key: K) -> V ! CacheErr[E] uses clock
   ```
3. **Untestable timing semantics from the spec alone.** Stale while revalidate
   and in flight invalidation tests depend on exactly when spawned tasks run
   versus when the fake clock fires timers. The spec gives auto advance rules
   but not spawn ordering, and the fake network cannot add latency. Since the
   checker cannot run programs, these tests compile but their timing
   assumptions are unverified.
4. **Arm typing with mixed success and failure.** Having to write `Ok(v)` in
   some arms and `return Err(...)` in others (gap 5) is noise compared with a
   single arm `v` / `Err(e)` rule.

## Next steps

- Run `xo test` on this folder once execution lands, to check the timing
  assumptions in the stale refresh and invalidation tests.
- Plant a deliberate type error inside a `test` block (for example
  `expect(cache.get(1)?) == "x"` in `cache_test.xo`) and run `xo check` to
  confirm whether test blocks are type checked.
- Probe gap 5 with a one-off file: `fn f(x: Int) -> Int ! Str { match x { 0 => 1  _ => Err("no") } }`.
