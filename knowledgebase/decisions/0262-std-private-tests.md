# 0262: Private-function tests inside std

Status: Accepted (owner, 2026-10-18, through the lead: approved as written); scheduled after decision 0261 stage (a) (Xo Lang 3); it lets the map_model and canonical_names goldens become module tests

## Context

A module's `_test.xo` files may call its private functions (spec/core.md
9.2: `regressions_test.xo` lives in the module itself for that reason),
but `xo test` runs the tests of the program's own modules only. std's
modules, and the native runtime under std/native and std/native/llrt
above all, have no Xo tests at all: they are tested from outside, through
Go tests that build whole programs (internal/llvm, internal/arm64, the
goldens) and through fuzzers.

That costs every time a std function needs checking on its own
(2026-10-18 examples):

- The Map model check (small maps against an insertion-ordered list)
  and the canonical header name check had to become goldens
  (map_model, canonical_names), with a hand-written generator and
  printed counts, because a property test had nowhere to run.
- scan.xo's text helpers (decision 0259), netpolicy.split_hostport, and
  hmac.pad were checked through HTTP and TLS programs or one-off scratch
  programs; their edge cases (a continuation byte at a slice end, a key
  of 20 bytes) are tested only as far as a whole program reaches them.
- A private helper cannot be called from a golden at all: 0259's tests
  went through Request.with_header to reach canonical_key.

## Proposal

1. A std module may have `_test.xo` files beside its sources:
   std/native/llrt/scan_test.xo tests scan.xo's private functions, with
   `test` blocks, property tests (`test "..." for x: T`), and `expect`,
   as in any module. They are never part of a program that uses the
   module: the checker loads a std module's test files only when that
   module is the one under test.
2. `xo test --std MODULE...` (MODULE a std path: `native/llrt/scan`,
   `http`) builds a test binary with that std module as the root, so its
   tests run with private access, on the default native backend or the
   one `--backend` names; `--cases`, `--seed`, `--run` as usual.
3. internal/stdtest (Go) runs every std module that has tests, on each
   backend the tier names (-short: the host's native backend; the merge
   tier: LLVM and arm64; full: also wasm and Go where the module
   compiles there), so the tests are part of the gates.
4. The runtime's own modules (llrt) are tested on the native backends
   only; a module that std/rt (the Go backend) implements in Go has its
   Go tests there.

## Consequences

- Model tests and edge-case tests of std live next to the code; goldens
  go back to being programs.
- The test binary of a std module links the native runtime it may itself
  be part of (scan.xo is used by the runtime's own HTTP code): the build
  must not duplicate the module, which the root-module build already
  handles for user modules.
- Test files are part of std's source key, so a test-only change
  rebuilds xo's cached copies once; they could be excluded from the key
  if that cost shows.

## Alternatives

- Keep testing std through programs (today): no private access, and
  every edge case costs a whole program.
- Make the functions pub for testing: widens std's surface (decision
  0207's llrt modules are internal for a reason).
- Go-side unit tests that call std functions through `internal.crypto_op`
  style dispatchers: they need a dispatcher entry per function and see
  only what the dispatcher exposes.
