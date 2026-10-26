# A `std/NAME` module's test file takes a different path from a native module's

Status: Observed (2026-10-25), fixed the same day (decision 0262's
Implementation).

## Symptom

`xo test --std-module=prelude` over a `std/prelude_test.xo` holding

```
test "t" {
  var xs = [3, 1]
  xs.push(4)
  expect(xs.len() == 3)
}
```

failed with XO1201 at every list literal and every call on the list:

```
std/prelude_test.xo:2:12: error XO1201: not available on the native backends
  (arm64, amd64, LLVM) yet: this list literal (support is planned)
std/prelude_test.xo:3:3: error XO1201: ... this call (support is planned)
```

The same code compiled and ran in a program, and the same test file under
`std/native/prelude_test.xo` (`--std-module=native/prelude`) passed. With
a function of the module called from the test, the failure moved to
XO1201 "the std function `probe.seven`". Option, Map and `Some(1)` failed
the same way; an Int comparison and `"ab".len()` did not, which is what
made it look like a list bug.

## Cause

Two gates skip std modules, and a `std/native/NAME` module is not one to
either of them, while a `std/NAME` module (the prelude, and every std
module whose implementation is a native module) is.

1. `compiler/check/dump.xo` `run` checks the bodies of the user modules
   and of `check_bodies_of`, never of an `is_std` module (`std_test` was
   not added). A test file parsed into `std/NAME` therefore had every
   body built from unchecked syntax: no name lookup, no type, nothing.
2. `compiler/ssa/call.xo` `call` sends any call into an `is_std` module to
   the std API lowerings (implementation module, io, prelude intrinsics),
   which end at `std_call`'s XO1201. A function of the module under test
   with a body of its own has no implementation module to lower to.

## Fix

The run's root std module is the module under test, so both gates take it:
`run` checks `c.mods[m].root`, and `call` builds a function of it from its
body, after the implementation and prelude lowerings so `List.push`,
`expect` and the rest still lower as they did. `internal/stdtest`'s
TestStdModuleTestBodies holds both halves over a copied std with a probe
module.

## Checking a new std test location

Before adding a test file at a new path (a `std/NAME_test.xo` beside a
shim module, say), run it once on arm64 and once with `--backend=llvm`:
an unchecked test body fails loudly now, but a *lowering* gap (a name the
std API path answered before) shows up only when the test calls into the
new location. The `std/native/...` shapes stay the cheap ones: they were
never affected.
