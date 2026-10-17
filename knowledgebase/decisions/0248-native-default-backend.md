# 0248: Native is the default backend

Status: Accepted (owner, 2026-10-16; implemented the same day by xo-fa)

## Context

The Go backend was the default of `xo build`, `run` and `test`; the
native backends (arm64, LLVM) needed `--backend` or `--no-gc`. Since the
native backends build every golden, link with Xo's own linkers on macOS
and Linux (decisions 0182, 0243) and are the self build's, a new user's
first build should not need the Go toolchain. The owner approved making
native the default as part of the "native out of preview" milestone.

## Decision

- With no `--backend`, `xo build`, `run` and `test` use the host's native
  backend: arm64 on arm64 hosts, LLVM on x86_64 hosts (selfhost/xo
  main.xo). `xo asm` keeps arm64.
- `--backend=go` stays. A program with a `use go` package and no
  `--backend` is a usage error naming the package and saying to add
  `--backend=go` (no silent switch).
- The flags only the Go backend has choose it when no backend is given:
  `--hybrid`, `--holes`, `xo test --explore`, `xo run --seed` and
  `--record`. `--race` chooses LLVM (ThreadSanitizer; arm64 has no race
  detector).
- LLVM as the default with no clang: the error names `xcode-select
  --install`, the distribution's clang, `XO_CLANG`, or `--backend=go`.
- Contract fuzzing (core 9.2) and `xo dump` (11.4) are the Go backend's
  for now. A native `xo test` skips the contract fuzz tests and prints
  `MODULE: contract fuzzing needs --backend=go: N contracts not fuzzed`
  per module; `xo dump` of a native program says it needs a Go backend
  build. Native contract fuzzing is a priority backlog item (contracts
  are a core feature).
- Tests: internal/xotool's Build names `--backend=go` when a test gives
  no backend (they were written for the Go default); cmd/xo's dump and
  regression tests name it too; the help golden is regenerated.

## Consequences

- `xo build` needs no Go toolchain on arm64 hosts (Xo's assembler and
  linkers) and clang on x86_64 hosts.
- `xo test` runs on the real scheduler natively; `--explore`, seeds of
  `xo run`, and contract fuzzing need `--backend=go` until the native
  side has them.
- Xo Lang 3 rewrites the website's getting-started page around it.

## Alternatives

- Pick the Go backend when a test package has contracts: a backend that
  changes with the code is surprising; the explicit line per module says
  what was not checked.
- Keep the Go default until native contract fuzzing exists: users would
  keep needing Go for the common path.
