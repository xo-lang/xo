# A type error in std/native faults the compiler instead of being reported

Status: Observed (2026-11-17, xo-lang-2, task #222; xo at 4a898f31e); fixed in 500279963

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

One of the errors was a parse error in a lazily checked body (the `is`
binding, XO0165). In `body_of`, a lazy body that did not parse was
skipped only after `param_bindings` ran, and with a `var` parameter
param_bindings indexed the missing bindings and faulted, before the
check errors were printed (Lang 1's diagnosis, 2026-11-17). The other
errors (`List.extend`, `??` on a Result) printed once the fault was gone.

## Fix

Fixed in 500279963 (Lang 1): the unparsed lazy body is skipped before
param_bindings; test cmd/xo TestDepLazySyntaxErrorVarParam. With an older
xo, when `xo test --std-module` or a std build faults in ssa, look for a
parse or type error in the newest std/native code first.
