# 0083: `--no-gc` builds and the XO1201 check

Status: Accepted (2026-10-09). Implemented in `cmd/xo` (`--no-gc` on
build, run, test, check), `internal/build/nogc.go` (`NativeBackend`,
`CheckNoGC`), `internal/ssa/nogc.go` (`CheckNoGC`, the diagnostics),
`internal/ssa/build.go` (`unsupported`, `impossible`). Tests:
`internal/build/nogc_test.go` (TestNoGCCheckAgrees), `cmd/xo`
(TestNoGC), `internal/llvm/testdata/unsupported/*`.

## Context
The native backends (decisions 0046, 0049, 0068 to 0082) build a growing
subset of Xo without a garbage collector, but a program outside the
subset failed late, inside the SSA builder, with plain text errors
("the SSA builder does not support ...") and no way to tell whether the
gap will close. The backend had to be named (`--backend=llvm` or
`arm64`) and the host decided which one works. Agents need the answer
as stable diagnostics before anything is generated.

## Decision
1. **`--no-gc`** on `xo build`, `xo run`, `xo test`, and `xo check`
   selects a native backend: arm64 on darwin/arm64 and linux/arm64
   hosts, LLVM otherwise. `--backend=llvm` or `--backend=arm64` refines
   it; `--backend=go` with `--no-gc` is a usage error.
2. **XO1201**, a new code in a new `12xx` group (native builds without a
   garbage collector): "not available without a garbage collector
   (`--no-gc`)", at the construct's site, naming the feature, with the
   support class in the message and in a note: *planned* (the native
   backends will grow it) or *impossible* (a call of a `use go` package
   or a value of its types: Go code needs Go's runtime).
3. **The check is the builder.** Every native build (with `--no-gc` or a
   named native backend) runs internal/ssa's builder as a check after
   type checking and before code generation; each place the builder
   cannot lower something records it (`unsupported`: planned,
   `impossible`: never) with its span, and those records are the
   diagnostics. No separate list of supported features exists to drift.
   An error recorded inside the span of an earlier one is dropped
   (core 10.2: never cascade). TestNoGCCheckAgrees requires, for every
   native golden, unsupported, benchmark, selfhost, and example program,
   that the check reports nothing exactly when both the LLVM and the
   arm64 code generators succeed.
4. **What the check adds beyond the builder's gaps:** an `https` URL
   written as a literal in a std/http client call (`get`, `post_json`,
   `new_request`) is XO1201 (TLS, planned; HTTP/2 needs TLS too). A URL
   computed at run time still fails with `ClientErr.Protocol` (std.md
   4.5). A test build reports each test block until native test builds
   exist.
5. `xo check --no-gc` checks programs only (the builder starts at
   `main`); a library is not checked by it.

## Consequences
- `xo build --backend=llvm` of an unsupported program now fails with
  XO1201 diagnostics (exit 1, JSON with `--json`) instead of a plain
  error.
- The check builds the SSA once more than a plain native build; the cost
  is a few milliseconds per program.
- Adding native support for a construct removes its XO1201 with no
  other change; a backend lowering that rejects what the builder accepts
  fails TestNoGCCheckAgrees.

## Alternatives
- **A separate allow list of std functions and constructs in the
  checker**: readable, but it drifts from the builder; the builder is
  the only place that knows what lowers.
- **Report through the existing build error text**: no stable code, no
  span, no support class; agents could not act on it.
- **Warnings instead of errors**: the program would not build anyway.
