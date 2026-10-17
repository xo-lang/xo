# 0247: A native build checks the runtime's bodies as it builds them

Status: Accepted (lead, 2026-10-16, for the playground of decision 0242;
xo-4b)

## Context

A native build checks the program in native mode: the implementation
modules of std (std/native/*.xo) and the runtime modules
(std/native/llrt/*.xo) are loaded with it, and until now every function
body of every loaded runtime module was checked before the SSA builder
started. A program reaches a small part of that runtime: the SSA builder
already builds only what is reachable from main (its work queue), but
the checker had checked everything.

The checker's share of a build is large. Profiles
(bench/results/lazy-runtime-bodies-2026-10-16.md):

- native `xo build --backend=arm64` of a hello program: the checker
  47.9% of the build, its body checks 24.1%; the SSA builder 7.4%;
- the wasm build of the compiler running the same build in a browser
  tab (decision 0242): the body checks of the runtime were 61% of the
  build and the flow passes over them 11%; the SSA builder under 1%.

## Decision

- **Lazy runtime bodies.** In a build (`Checker.lazy`, set by
  selfhost/xo check_program for a native build and by selfhost/wasm
  play_build), `check_module` skips the function bodies of the native
  modules (`Module.native`): it records each function in
  `Checker.lazy_fns` and checks only its parameters' default values (a
  caller's build reads them). The user's modules, and every
  declaration, constant, struct and derive of the native modules, are
  checked as before.
- **The SSA builder checks a body when it first builds it.**
  `Builder.body` and `Builder.body_of` call `Checker.check_lazy(fo)`,
  which checks the body as `check_module` would have (the same
  `check_fn_decl`, then run's fix of closure results that nothing
  determined). An error there is std's own: the build stops with it
  ("the runtime does not check").
- **What a lazy body skips:** the flow passes (moves, lost and stored
  updates, critical sections), effect obligations, the real-time walk
  and unused name and import reports. They only report diagnostics, and
  std's are covered below. `check_unused_imports` and `check_realtime`
  skip the lazy modules.
- **std stays fully checked.** `xo check --no-gc` (and the checker's own
  CLI) check every body, eagerly, as before. TestStdChecks (cmd/xo,
  -short) checks a program that imports every std module (in the
  toolchain's namespace, so std/internal is visible) with no
  diagnostics, and proves the program reaches every runtime file with a
  body: in a copy of std with a function that does not check appended
  to each of them, each one is reported (135 files on 2026-10-16).
- **Effects.** Checking a body can read a dependency's xo.mod (the
  effect ceilings, check_ceiling), so `Checker.check_lazy`,
  `Builder.build` and the `ssa.build_program*` entry points declare
  `uses fs.read`; their callers are the drivers, which have it.
- `XO_CHECK_EAGER=1` makes a native build check every body first, as
  before (for comparisons).

## Consequences

- Diagnostics and generated code are the eager build's: every golden
  (180) gives the same arm64 binary and the same LLVM IR, with the same
  output, and selfhost/xo the same binary
  (bench/results/lazy-runtime-bodies-2026-10-16.md).
- Build times on the Mac: hello 947 to 622 ms (-34%), the 180 goldens on
  arm64 289.6 to 227.6 s summed (-21%), selfhost/xo unchanged (about
  16 s; it reaches most of the runtime, and its own modules are checked
  in full either way).
- An error in a runtime body that no test program reaches is reported by
  TestStdChecks and `xo check --no-gc`, not by a build.
- The order in which bodies are checked changes (a body is checked when
  the builder reaches it), so checker objects and types are numbered in
  a different order; nothing the builder emits depends on those numbers
  (the comparison above).

## Alternatives

- **Reachability in the checker** (a worklist over the calls recorded
  in checked bodies, before the build). It must know every implicit
  call the builder makes (Display for interpolation, Eq and Hash for
  keys, iteration, interface tables, exports), which the builder's own
  queue already knows. Rejected for that duplication.
- **Check bodies lazily for the playground only.** The native build has
  the same waste; one mechanism for both.
- **Snapshot a checked std** (decision 0242's alternative): restore a
  checker with std already checked. Far more machinery (serializing the
  checker) for the same saving.
