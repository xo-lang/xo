# 0246: The SSA builders fold the build's target and test mode

Status: Accepted (2026-10-16, Xo lang 3, assigned by the lead: "a darwin
binary carries no linux.xo or windows.xo ... non-test binaries carry no
test world")

## Context

Spec core 12.2 and decision 0005 say only the arm a `match` on `target`
takes is generated, and every arm is checked. The SSA builders lowered
every arm (gotchas dead-target-arm-still-links.md,
arm64-match-target-not-folded.md, wasm-target-os-is-the-host.md); LLVM's
optimizer dropped the dead ones, the arm64 emitter did not. A darwin
binary carried linux.xo and windows.xo (13 to 22 KiB of code) and the
test world (testworld.xo, twrun.xo: 6 to 21 KiB), whose switches
(`testworld.fake(...)`, `testworld.on()`) are words read at run time
(bench/results/binary-size-native-2026-10-16.md).

## Decision

selfhost/ssa (expr.xo, target_test and target_arm):

1. `if` whose condition the build decides lowers only the branch it
   takes, and a block stops lowering after a return (or another
   statement that leaves it), which Xo, with no goto, never reaches. The build decides `target.os` and `target.arch` compared
   (`==`, `!=`) with a TargetOs or TargetArch variant, the OS layer
   number (native/llrt/sysio.layer and netconn.os) compared with an
   integer literal, `!` of such a
   test, and `&&` and `||` of them (also when the left side alone
   decides, as evaluation would short circuit).
2. A `match` on `target.os` or `target.arch` lowers only the first arm
   whose pattern (a variant, an or-pattern of variants, or `_`) matches;
   an arm with a guard or another pattern before it keeps the old
   lowering.
3. Outside a test binary (Builder.tests false), a call of the test
   world's switches, `on` and `fake` of native/llrt/testworld and
   native/llrt/twflags, with constant arguments, is false for (1).
4. The checker still checks every arm, so a broken arm for another
   platform still fails on every host (decision 0005).

## Consequences

- Sizes (darwin/arm64, stripped, against seed 0.18.0 at the same commit):
  hello 646,616 to 580,344 bytes (minus 10 percent), https_get
  2,447,136 to 2,133,232 (minus 13), json_server 2,727,600 to 2,529,424
  (minus 7), notes-service 3,173,424 to 2,942,224 (minus 7). Linux and
  Windows code in a darwin binary went from 13.5 KiB to 1 KiB, the test
  world from 5.8 KiB to 1.8 KiB; the rest is other platforms' arms
  inside shared functions and code after early returns.
- The same change makes `-s` drop local symbols (selfhost/macho
  LinkOpts.strip, `-Wl,-x` on darwin and `-s` on Linux cc links): hello
  586,580 bytes with `-s`, against 701,012 before (where `-s` only
  dropped the dSYM).
- Code that only another platform's arm or the test world reaches is
  not built or linked. Runtime choices made at run time (a word set at
  start, as sysio.xo's clock choice) are not affected and should become
  `target` tests where the platform is all they depend on.
- arm64 no longer emits a compare chain for a `match target.os` in hot
  paths (gotcha arm64-match-target-not-folded.md).
- The wasm backend's `target.os` is still the host's (gotcha
  wasm-target-os-is-the-host.md); folding keeps the same arm the
  program took at run time before.

## Alternatives

- An SSA pass that folds branches on constants: more general, but the
  arms would still be built (their calls declared, their functions
  reached) before the pass; folding while building keeps them out of
  the program entirely.
- A new runtime intrinsic for test mode: needs the owner's approval
  (new intrinsics do), and the builder already knows the mode.
