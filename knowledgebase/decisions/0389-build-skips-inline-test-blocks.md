# 0389: `xo build` and `xo run` skip inline test blocks

Status: Accepted (owner, 2026-10-27, via the lead; built by Xo Lang 1)

## Context

Core 9.2 said test blocks "are compiled only by `xo test`", but a build still declared and
type-checked them, and so loaded std/testing for their predeclared `os`, `expect` and `testing`.
std/testing imports std/http, sql, log and json, which bring their implementations and the HTTP,
TLS, SQL, flate and DNS runtime modules. 408ad9b8c (ADR 0380 section) stopped loading std/testing
for a build with no test block (llvm-fib edit 282.9 to 243.8 ms); a program with one still paid
for it. The oracle that skipped std/testing (Xo Lang 1, 2026-10-27) measured 6.5 ms on the HTTP
server's edit, whose main.xo has a test block, and up to 36 ms on programs whose test blocks are
their only use of std/testing's imports.

Decision 0348 already gives `_test.xo` files this treatment: a build does not read them.

## Decision

`xo build` and `xo run` (Checker.skip_tests) drop every `test` declaration from a file's tree when
it is parsed (check/load.xo parse_into): a test block is neither declared nor type-checked, and a
build never loads std/testing for one. An error in a test block fails `xo check` and `xo test`,
not a build. `xo test`, `xo check`, `xo vet`, `xo fix` and the language server check every test
block as before. Core 9.2 and its 0.10 changes list say so.

Precedent: Rust compiles `#[cfg(test)]` items, inline test modules included, only under
`cargo test`, and a type error in one does not fail `cargo build`; Go's `go build` does not compile
`_test.go` files.

## Consequences

- A build reads less for a program with test blocks. The HTTP server's edit parses 179 files
  instead of 197 (check 56.8 to 54.8 ms, ssa 95.6 to 93.0), but its wall time did not move beyond
  noise in a quiet --ab against the parent (n=10, 588.3 to 587.6 ms, medians 583 to 591): its own
  std/http import already brings most of what std/testing did. The oracle's up to 36 ms is for
  programs whose test blocks are the only reason std/testing's imports were loaded.
- A test block that no longer type-checks is found by `xo check` or `xo test`, not by `xo build`:
  an agent's edit loop that runs only `xo build` can leave a broken test. `xo check` is the
  checking command (and what the language server runs).
- check/load.xo load_testing_if (408ad9b8c) is gone: a build never loads std/testing.
- cmd/xo TestBuildInlineTestBlocks asserts the new behavior (a broken block builds and runs, fails
  check and test; a block adds no parsed file to a build).

## Alternatives

- Keep checking test blocks in a build (the owner's first call, 2026-10-27): 6.5 ms on HTTP was
  judged not worth a semantics change, then reversed the same day for the consistency with 0348
  and the larger gain on test-block programs.
- Check test blocks in a build without std/testing (a light core of it): std/testing's fakes use
  http types (TestOs.net is a FakeNet), so it cannot be split without moving them.
