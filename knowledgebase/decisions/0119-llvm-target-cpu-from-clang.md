# 0119: LLVM backend code is compiled for clang's own default target (LSE atomics)

Status: Implemented (2026-10-10). Code: `internal/build/llvm.go`
(`ClangTargetAttrs`, `parseTargetAttrs`), `internal/llvm/llvm.go`
(`Options.TargetAttrs`, `withTargetAttrs`). Tests: TestParseTargetAttrs,
TestLLVMTargetAttrs (internal/build; passed on darwin/arm64, and in the
linux/arm64 and linux/amd64 containers), the LLVM goldens on all three.
Numbers: `bench/results/native-runtime-2026-10-10.md`, section 3.

## Context

The generated IR has no target triple or CPU: clang supplies the triple
and LTO compiles every function that has no `target-cpu` attribute for
LLVM's generic CPU of the triple. The C runtime, compiled by the same
clang from source, gets clang's default CPU and features (`apple-m1`
with LSE on darwin/arm64). So the generated code's shared reference
counts (decision 0058) were exclusive load and store loops (20 in a
small spawning program, none LSE) while the runtime's own atomics and
the arm64 backend on Darwin (`stadd`, `ldaddal`) used LSE. Functions
with different target features also cannot be inlined into each other
by LTO.

## Decision

1. The build asks clang once per compiler for its defaults (`clang -###
   -c -x c /dev/null`: the cc1 line's `-target-cpu` and every
   `-target-feature`) and adds them as `"target-cpu"` and
   `"target-features"` to the attribute groups of every generated
   function definition (`#1`, `#4`).
2. No CPU of our own choosing: the generated code targets exactly what
   the runtime's C targets on that host. darwin/arm64: `apple-m1`, LSE
   inline. Linux on arm64 (Debian clang 19): `generic` with
   `+outline-atomics`, so atomics call libgcc's or compiler-rt's
   `__aarch64_*` helpers, which use LSE when the CPU has it and
   exclusive loops on ARMv8.0 (the baseline fallback). x86-64: `x86-64`.
3. Callers of `llvm.Generate` that leave `TargetAttrs` empty (unit tests
   that compile IR themselves) keep LLVM's generic CPU.

## Consequences

- darwin/arm64: no exclusive loops left in generated code (0 of 20; 72
  LSE instructions in the spawn program). bench/perf/http kept alive:
  112.9k to 116.8k req/s, p99 960 to 920 us (3 interleaved rounds); the
  concurrency and compute benchmarks within 2 percent (M4's exclusive
  loops are cheap without contention).
- The arm64 backend still writes exclusive loops on Linux (it has no
  run time dispatch); outline atomics for it would mean calling the same
  helpers from its assembly (next step if Linux arm64 contention shows).
- A build with a different clang, or one configured for another default
  CPU, compiles the generated code for that CPU too, as it does the
  runtime; the build cache key already covers the IR, which now
  contains the attributes.

## Alternatives

- **`-mcpu`/`-march` on the clang command line**: applies to C sources,
  not to the LTO code generation of an IR file without function
  attributes on every linker (ld64 takes its CPU from the bitcode).
- **A fixed `+lse`**: wrong on ARMv8.0 Linux machines (Raspberry Pi 4,
  Graviton 1), which fault on LSE instructions.
- **Outline atomics everywhere**: a call per atomic where Darwin can
  inline LSE.
