# Storing closures that have effects

Last verified: 2026-11-05 (decision 0522; the example is the backend
golden internal/llvm/testdata/golden/effect_threaded).

A closure kept in a struct field or behind an interface performs its
effects when it is called, so the type that holds it says which effects
those are. Coming from Go or TypeScript, the habit is to store a `func`
and call it anywhere; in Xo the effects travel with the type as an effect
parameter, and every function that calls the closure lists them.

## The threaded form

```
pub struct Request[fx: Effects] {
  pub name: Str
  pub sign: fn(Str) -> Str uses fx
}

pub fn send[fx: Effects](req: Request[fx]) -> Str uses (clock, fx) {
  let attempt = fn() -> Str uses (clock, fx) { (req.sign)(req.name) }
  attempt()
}
```

- `Request[fx: Effects]`: the struct takes the closure's effects as a
  parameter.
- `send[fx: Effects](req: Request[fx]) uses (clock, fx)`: `send` names
  them and declares that it performs them. A caller passing a request
  whose `sign` reads the environment calls `send` with `fx = env`, and
  its own `uses` must include `env`.
- A closure inside `send` that declares its effects lists `fx` too.

Leaving the argument out (`req: Request`) also works where nothing
declares effects: the omitted argument is an implicit effect variable of
the function (core 6.2). It cannot be named in a `uses` list, so a
`uses (clock)` closure that calls `req.sign` is XO0701 "performs
`Request.fx`"; the error's notes give the threaded form.

## Interfaces

An interface whose implementations may have effects takes an effect
parameter, and a list of providers names the effects of the list:

```
pub interface Provider[fx: Effects] {
  fn token(self) -> Str uses fx
}

fn (p: Fixed) token() -> Str { p.value }
fn (p: FromEnv) token() -> Str uses env { p.env.get(p.key) ?? "" }

let ps: List[Provider[env]] = [FromEnv{key: "TOKEN", env: os.env}, Fixed{value: "x"}]
```

A pure implementation fits any `Provider[fx]`; one that performs more
than the argument allows is XO0701 "... allows only `env`, but
`T.token` performs `net`".

## Errors on the way, and what they mean

| Diagnostic | Meaning | Fix |
|---|---|---|
| XO0701 "the closure declares `uses clock`, but performs `Request.fx`" | an omitted effect argument cannot be listed | name it: `send[fx: Effects](req: Request[fx]) uses (..., fx)` |
| XO0702 "unknown effect `gx` in an effect argument" | the effect variable is not declared | add `[gx: Effects]` after the function's name |
| XO0202 "`?` propagates `F`, which does not convert to `E`" | inside a closure, the closure's error type is the target | `.map_err(fn(e: F) -> E { ... })?`, or a `from F` variant on `E` |
| XO0401 "expected `List[Int]`, found `Result[List[Int], Str]`" on `map` | `map` over a closure that can fail stops at the first error | add `?` |
| XO0502 "assignment mutates `n`, which is captured by a closure" | closures capture by value (decision 0003) | return the new value, use `for` or `fold`, or share a `Mutex` or `Atomic` |
