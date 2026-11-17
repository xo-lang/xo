# A type error in std/native faults the compiler instead of being reported

Status: Observed (2026-11-17, xo-lang-2, task #222; xo at 4a898f31e)

## Symptom

`xo test --std-module=toml` printed only a compiler fault:

```
fault: index 0 out of range for List of length 0 at ssa/build.xo:3703:25
  ssa.Builder.param_bindings (build.xo:3703)
  ssa.Builder.body_of (contract.xo:128)
```

An xo built to name the function showed `strict_missing`, a new
std/native/toml.xo function with a `var` parameter. Moving code around
and dropping every `var` parameter did not help. `xo vet --std-module=toml`
said "the program has 4 check error(s), not repeated here" and did not
list them.

## Cause

The std module had ordinary type errors (`List.extend` does not exist,
`??` on a Result, a binding after `is`). The test build went on to SSA
lowering anyway, and a function whose check failed had no parameter
bindings, so `param_bindings` indexed an empty list. When the
fault was gone the same command listed the errors (XO0601, XO0406,
XO0165).

## Fix

When `xo test --std-module` or a std build faults in ssa, look for a
type error first: remove or stub the newest std/native code until the
errors print, or read `xo vet --std-module=M`'s count. Reported to
Lang 1: the build should stop at check errors and print them.
