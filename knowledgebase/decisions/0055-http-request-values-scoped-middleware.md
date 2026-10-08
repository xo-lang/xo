# 0055: Typed request values, failing and scoped middleware, Listener.addr

Status: Accepted (2026-10-08)

## Context
The notes-service real application (`bench/results/real-app-2026-10-08.md`,
gaps 1 to 3 and 7) showed three holes in std/http:

- A middleware could pass the authenticated user to handlers only through a
  header it wrote itself, which is stringly typed and spoofable unless the
  middleware always overwrites it and guards every route
  (`gotchas/middleware-identity-header-spoofing.md`).
- `wrap` was router global and its middleware could not fail, so the public
  `/health` route was exempted by comparing `req.path`, and the middleware
  built its own 401 and 500 bodies instead of reusing the section 4.2
  mapping that handlers get.
- `Listener` had no methods: `listen(":0")` could not report its port, so
  tests used a fixed address and a program could not log what it bound.

## Decision
- **Typed request values.** `http.Key[T]` (a handle), `http.key[T](name)`,
  `Key.name()`, `Request.with_value[T](k, v) -> Request`, and
  `Request.value[T](k) -> T?`. A key is identified by its name and its type
  `T`, so a key can be rebuilt anywhere (`fn user_key() -> http.Key[User]`)
  without a global, and a value is always read at the type it was stored
  with. Values are process local: never sent or received, not part of
  `==`, `hash`, or `debug`. Runtime: a persistent linked list in a hidden
  field of `Request` (`XValues`), keyed by name and `reflect.Type`.
- **Failing middleware.** `wrap[E: Error, fx: Effects](m: fn(Request, Next)
  -> Response ! E uses fx)`. `Err(e)` is mapped exactly like a handler's
  error (section 4.2); a middleware that cannot fail infers `E = Never`, so
  existing code is unchanged. `Next` stays infallible.
- **Scoped middleware.** `Router.mount(prefix, sub)`: copies the routes of
  `sub` under `prefix`; each mounted route runs the outer router's
  middleware, then `sub`'s, then the handler. Unmatched requests do not
  reach `sub`'s middleware. One route is guarded by mounting a router
  that holds only it.
- **Addresses.** `Listener.addr()` and `Server.addr()`: the bound
  `host:port`, unspecified hosts reported as loopback so the result is
  dialable. The address is part of the recorded `net.listen` result, so
  replay returns the same string. The fake network's `:0` ports start at
  49152.
- Also `Request.with_header(name, value)`, symmetric with `Response`.

## Consequences
- The header workaround and its gotcha are obsolete; notes-service uses a
  mounted, failing auth middleware and `req.value`.
- `Router` holds no tree: `mount` flattens routes at registration, so
  dispatch is unchanged and route precedence (most literal segments) spans
  mounted and direct routes alike.
- Request scoped arenas (`--arenas`): a mounted route with middleware is not arena
  eligible (its middleware sees the request), same as global middleware.
- Recordings made before this change have no `addr` on listeners; replay
  then returns `""` from `addr()`.

## Alternatives
- Keys compared by identity (each `key()` call a new key): needs a global
  to share one key, and Xo top level `const` initializers must be compile
  constants (core 3.1). Rejected.
- A context map `Map[Str, Debug]` on `Request`: still stringly typed at the
  read site. Rejected.
- `group(prefix) -> Router` returning a view that registers into the
  parent: needs shared mutable state between router values, against value
  semantics (core 5). `mount` keeps `Router` a value.
- `handle_with(mw, pattern, h)` for single routes: covered by mounting a
  one route router; may be added later as sugar.
- `Next` returning `Response ! E`: would force every middleware to handle
  inner errors that are already mapped. Rejected.
