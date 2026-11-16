# A struct's effect parameter is not inferred from closure arguments

Status: Observed (Xo Lang 3, 2026-11-16, xo 0.52.0 at 4bc58f27d)

## Symptom

```
let port = term.func_port(fn(max: Int) -> Bytes? ! Str uses sync { ... }, fn(b: Bytes) -> () ! Str uses sync { ... })
let t = term.new_terminal(port, "> ")
```

fails with `XO0701: this value's type has effects sync, but the expected
type term.Port[()] is pure` at the `new_terminal` call (bench/perf/term,
while porting decision 0684), although both closures declare `uses sync`
and `func_port[fx]` returns `Port[fx]`. Adding `sync` to the enclosing
function's `uses` does not help.

## Cause

The `let` binding takes `Port[()]` for the call's result: the effect
variable `fx` is not bound from the closure literals' `uses`. Not yet
narrowed further; reported to Xo Lang 1.

## Fix

Annotate the binding: `let port: term.Port[sync] = term.func_port(...)`,
or return it from a function whose result type names the effects
(`fn port_of(m: Mock) -> term.Port[sync]`, as the term corpus does).
