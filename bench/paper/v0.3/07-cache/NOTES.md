# Task 07 (loading cache): paper test notes, spec v0.3

Files:

- `xo.mod`
- `cache/cache.xo`: generic `Cache[K, V, E, fx]`
- `cache/cache_test.xo`: the six required cache tests
- `itemsvc/itemsvc.xo`: upstream client, HTTP routes, `main`
- `itemsvc/itemsvc_test.xo`: service tests (cache hit, invalidate, 404, 502)

Design in one paragraph: all mutable state lives in one `Mutex[State]`. `get`
decides what to do under the lock (`Plan`: ready, stale, wait, lead) and acts
outside it. A load is a `Flight` holding a `Chan[()]` that is closed when done
plus a `Mutex` holding the outcome, so any number of waiters can block on
`recv()` and then read the shared result (a broadcast, since `close` wakes all
receivers). The leader runs the loader in its own task inside
`task.isolate(task.with_timeout(...))`, and a `defer` always publishes the
outcome and wakes waiters. `invalidate` removes the flight from the map, so
`finish` sees the flight is no longer current and does not store. LRU uses the
insertion ordered `Map`: a touch removes and re-puts the key, so the first key
is the oldest. Stale refreshes are sent on a channel to `run_refresher`, which
the owner runs with `spawn_background`.

## 1. Spec gaps

1. **Multiple constraints on one type parameter** (core 2.7, grammar
   `TypeParam`). Map keys need `Eq` and `Hash`; the grammar allows only one
   constraint. Guessed `K: Hash` implies `Eq`.
2. **Effect variables on structs** (core 6.2, 15). `StructDecl` takes
   `TypeParams`, and `TypeParam` allows `: Effects`, but every example is a
   function. A cache must store the loader, so I used
   `struct Cache[K, V, E, fx: Effects] { load: fn(K) -> V ! E uses fx }`.
3. **Concrete effects as a type argument** (core 6.2). To name the cache type in
   the service I wrote `cache.Cache[Int, Item, UpstreamErr, net.dial]`. Nothing
   says an effect label (or a set like `(clock, net)`) can fill an effect
   variable in type position. Also, there `net` is both a parameter name and an
   effect; 6.1 says namespaces are separate, but in type argument position the
   parser cannot tell.
4. **Type parameters of generic receivers** (core 3.4). `fn (c: Cache[K, V, E,
   fx]) get(...)`: are `K` etc. declared by the receiver, and do the struct's
   constraints carry over? std.md writes `fn (xs: List[T]) map[U]` so I
   assumed yes.
5. **Closure with a `Result` tail: failing or infallible?** (core 3.3, 2.6).
   `fn() { load(key) }` could be `fn() -> V ! E` or `fn() -> Result[V, E]`.
   The scope example `s.spawn(fn() { fetch_user(db, id) })` implies the
   former; I relied on it for `task.isolate` and `task.with_timeout`.
6. **Cancellation of the caller of `with_timeout`** (std 7). `Timed[E]` has no
   `Canceled` variant. If the leader's own task is cancelled mid load, what does
   `with_timeout` do? I assumed it unwinds (like `sleep`) and used `defer` so
   waiters still wake, with a "load canceled" `Crashed` outcome.
7. **Fault inside `with_timeout` inside `isolate`** (core 8, std 7). Is a fault in
   the timed child converted by the outer `isolate`, or does the child scope
   rethrow it in a way `isolate` sees? Assumed `isolate` catches it.
8. **Holding a scope beyond a call** (core 7.1). There is no type for the scope
   handle `s`, so a struct cannot keep one to spawn refreshes later. I used a
   channel plus a `run_refresher` daemon instead. Not a guess, but the task
   reads "start a background refresh" and the spec gives no direct way.
9. **No non blocking send** (core 7.3, std 2.7). `Chan.send` blocks when full and
   there is no `try_send`; `select` without `after` might work as a non
   blocking send only if it had a default arm, which it does not. So a stale
   `get` can block if nobody runs `run_refresher`. Sized the queue to
   `max_entries` and documented it.
10. **No `while let`** (core 3.5). Wrote `while { match ch.recv() { Some(j) =>
    ..., None => break } }`.
11. **`if` without `else` whose block has a value** (core 3.5). `if ok {
    got.add(1) }`: the block is `Int`, the `if` is said to be `()`. Assumed the
    value is discarded.
12. **Calling a function typed field** (core 2.2, 3.3). `c.load(key)` might
    resolve as a method call. I bound `let load = c.load` first. Not marked as a
    guess since it avoids the question, but the spec should say.
13. **Local module imports** (core 4.1). Guessed `use example.com/itemcache/cache`
    for a sibling directory in the same `xo.mod`.
14. **`Chan[()]`** (core 2.1). Unit as a channel element is not shown anywhere;
    assumed fine.
15. **FakeNet handlers cannot sleep** (std 8.1). The handler type is pure, so
    the "about 200ms upstream" fake cannot be expressed at the HTTP level. Moved
    the latency into the cache tests' loaders.
16. **Type inference of `Err(...)` alone** (core 2.7). `let want = Err(x)` has no
    `T`; I annotated the binding rather than guess.
17. **Is a timeout or crash a "loader error" for negative caching?** (task, not
    spec). I cache only `CacheErr.Load` errors; timeouts and crashes are not
    stored so "the cache works afterwards" holds immediately.

## 2. Habits

- Started to write `sync.WaitGroup` / `sync.Cond` style waiting for the single
  flight. Changed to a `Chan[()]` that is closed, plus a `Mutex` for the
  outcome (handles are shared across copies).
- Started to write `container/list` style doubly linked LRU with pointers. No
  pointers; used the insertion ordered `Map` and remove plus `put` to move a
  key to the end.
- Wanted `go c.refresh(key)` from inside `get`. Tasks cannot outlive a scope,
  so the refresh goes through a queue to a daemon task.
- Wanted `ctx context.Context` for timeouts. Used `task.with_timeout(clock, d,
  f)` and no context parameter.
- Wanted `recover()` for the panicking loader. Used `task.isolate`.
- Wrote `nil` / `None` checks like `if flight != nil`. Used `if let Some(f)`.
- Wanted `while let Some(job) = ch.recv()` (Rust). Changed to `while { match
  ... }`.
- Wanted a `type ItemCache = cache.Cache[...]` alias and a method value
  `up.fetch` as the loader. Used a closure `fn(id: Int) { up.fetch(id) }`; kept
  the full generic type in signatures.
- Wrote `fmt.Sprintf` / `{}` style formatting in error messages. Changed to
  `"${x}"`.

## 3. Friction

- **Constraint repetition.** Because `CacheErr[E: Display]` constrains `E`, every
  generic type that mentions it must repeat the constraint:
  `struct Entry[V, E: Display]`, `struct Flight[V, E: Display]`,
  `enum Plan[V, E: Display]`, `fn wait[V, E: Display](...)`.
- **Effect variable leaks into the type.** The loader's effects become part of
  the cache type, so every user must name them:
  `fn routes(items: cache.Cache[Int, Item, UpstreamErr, net.dial]) -> http.Router uses (clock, net.dial)`.
  Changing the upstream from `net.dial` to anything else ripples through every
  signature.
- **No detached work.** Structured concurrency forbids `go refresh()` from
  `get`, so stale refresh needs a queue and an owner managed daemon:
  `s.spawn_background(fn() { items.run_refresher() })`. Forgetting it does not
  fail to compile; it blocks `get` once the queue fills.
- **Broadcast needs two primitives.** One result for N waiters needs
  `done: Chan[()]` plus `outcome: Mutex[Result[V, CacheErr[E]]?]` and an
  `or_fault` on read, because there is no once cell or promise type.
- **Nested error wrapping from std/task.** Combining timeout and isolation gives
  `Isolated[Timed[E]]`, matched as
  `Err(task.Isolated.Failed(task.Timed.Failed(e))) => Err(CacheErr.Load(e))`.
- **Pure fake handlers.** `os.net.handle(..., fn(req) { ... })` cannot sleep on
  the fake clock, so the upstream latency in the task could not be faked at the
  HTTP layer.
- **Value semantics inside the lock.** Mutating through `var st` is pleasant, but
  helpers need a `var` parameter or a `var` receiver
  (`fn (var st: State[K, V, E]) touch(...)`), and I had to check by hand that
  nothing copied an entry into a `var` (XO0520).

## 4. Confidence

| File | Estimated remaining spec violations |
|---|---|
| `xo.mod` | 0 |
| `cache/cache.xo` | 2 to 4 (most likely: generic receiver syntax, effect var on struct, closure result inference, `Chan[()]`) |
| `cache/cache_test.xo` | 1 to 3 (timing assumptions about the fake scheduler, `if` value discarding, `expect` with `Metrics{...}` defaults) |
| `itemsvc/itemsvc.xo` | 2 to 3 (effect label as a type argument, local import path, `net` name collision in type position) |
| `itemsvc/itemsvc_test.xo` | 0 to 2 (`decode_json[Map[Str, Str]]` on an error body, `const` used in interpolation) |
