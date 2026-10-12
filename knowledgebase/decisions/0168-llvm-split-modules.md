# 0168: LLVM builds split into modules compiled in parallel

Status: Accepted (2026-10-12, llvm-split agent, on the lead's task),
implemented: `internal/llvm/modules.go` (`Split`), `internal/build/llvmsplit.go`
(`SplitConfig`, `LLVMSplitConfig`, `splitBuild`, `EnableTestLLVMSplit`),
`internal/build/llvm.go` (`buildLLVM` writes `prog.<i>.ll` and builds in
`llvm-split<n>-<lto>`), `build.Options.LLVMSplit`, `internal/selfhost`
TestMain. Tests: `internal/llvm` `TestSplitShape`, `TestSplitLinks`,
`TestGoldenSplit`. Evidence: `bench/results/llvm-build-time-2026-10-12.md`.

## Context

An LLVM build (`xo build --backend=llvm`) emits one textual IR module
and clang compiles it at -O2 with full LTO, on one core. For the self
hosted SSA builder (selfhost/ssa: 380 MB of IR, 2,569 functions, the
largest 127,552 lines) that is 372 s of compile to bitcode and 2,023 s
of LTO link, about 40 minutes, the largest serial step of the selfhost
full test tier. Everything before clang (check, SSA, IR generation)
takes 37 s.

## Decision

1. **Split by size, not by package.** `llvm.Split(ir, n)` places the
   program's functions (`x.*` and `xo_entry`) on n modules, largest
   first onto the lightest module, so the modules are within a few
   percent of each other in bytes. A per package split would be
   unbalanced: selfhost/ssa's `Builder` methods are most of its code.
   Every module gets the whole header (types, runtime declarations,
   attribute groups, metadata), a private copy of every constant it
   uses, an internal copy of every helper it uses (`xo.dup.*`,
   `xo.drop.*`, `xo.eq.*`: about 0.2 percent of the IR, mostly
   alwaysinline, so every module still inlines them), and a `declare
   hidden` of every function of another module it calls. A function
   another module calls changes from `internal` to `hidden`. The split
   is a pass over the text (0.5 s for 380 MB), deterministic, and
   leaves the generator unchanged; the self hosted emitter's IR
   (decision 0166, byte identical) splits the same way.
2. **Parallel clang, bounded.** `XO_LLVM_SPLIT=n` (or
   `build.Options.LLVMSplit`) compiles the n modules with at most
   `XO_LLVM_JOBS` clang processes at once across the whole process
   (default min(cores, 6)), then links the objects with the runtime.
   `XO_LLVM_LTO` picks `thin` (the default: bitcode modules, ThinLTO
   link) or `none` (native objects, the runtime compiled without LTO).
3. **Real builds keep one module with full LTO.** Unset
   `XO_LLVM_SPLIT` is today's build, byte for byte the same command.
4. **Test processes split.** `build.EnableTestLLVMSplit` (called by
   internal/selfhost's TestMain) sets 12 modules, 4 jobs, no LTO when
   the environment sets none: 512 s for selfhost/ssa instead of 2,394
   s (4.7 times faster), output 2 to 3 percent slower, which a test
   does not need. 12 modules and 4 jobs, not 6 and 6: a clang process
   on a sixth of selfhost/ssa peaks at 2.5 to 4.0 GB, six at once about
   20 GB of the machine's 24; on a twelfth at 1.4 to 2.3 GB.

## Consequences

- selfhost/ssa's LLVM build in the tests: 512 s instead of about 2,400
  s; the full tier loses about half an hour of serial clang.
- ThinLTO on 12 modules builds in 556 s with output as fast as full LTO
  (within the noise of three rounds), but its link runs on every core
  (3,670 s of CPU against about 1,860 s for no LTO): the right mode for
  a user who wants a fast build of a large program without giving up
  speed, not for a shared test machine.
- Without LTO the runtime is not inlined into generated code, as it is
  in a released build: a miscompile that only LTO inlining exposes
  shows in the goldens (single module, full LTO) but not in a split
  selfhost build. `XO_LLVM_SPLIT=0` (or `XO_LLVM_LTO=thin`) restores
  the release configuration for a test run.
- A split build has its own build directory (`llvm-split<n>-<lto>`),
  so a single module build of the same program may run beside it, and
  its own binary cache key.
- `usesTLS` reads a split object's IR (`prog.<i>.ll`) to decide on
  the TLS library.

## Alternatives

- **Faster single module builds** (-O1, no LTO): measured in the
  results file; one module stays serial and -O2 without LTO still took
  1,589 s.
- **Split in the generator** (one module per Xo package, declarations
  emitted per module): more code in fromssa.go and in the self hosted
  emitter that must stay byte identical to it, and unbalanced modules.
- **`llvm-split` or `-flto-partitions`**: Apple's toolchain ships
  neither llvm-split nor lld's `--lto-partitions`; ld64 runs ThinLTO
  backends in parallel only across modules, so one module stays serial.
- **Split real builds by default with ThinLTO**: output speed held on
  selfhost/ssa, but measured on one program only and the link uses
  every core; left as a next step (benchmarks of bench/ programs with
  `XO_LLVM_SPLIT=12`).
