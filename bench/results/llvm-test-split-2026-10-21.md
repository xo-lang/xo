# LLVM test build split: 8 modules, a module per file, or none (2026-10-21)

Which rule `xo test --backend=llvm` uses to split a test program's IR into modules whose objects
the back-end cache keeps (XO_LLVM_TEST_SPLIT; decision record in toolchain.md "Test build split").

## Method

- Tree: main at about 11:30 on 2026-10-21 (after 79116154) plus branch fa-llvm-split-files (the
  `files` mode), xo built from it with `go run ./scripts/xobuild`.
- Targets: `xo test --backend=llvm --std-module=native/json` (json) and
  `xo test --backend=llvm compiler/ssa` (ssa).
- Modes: `8` (files hashed into 8 modules), `files` (a module per source file, at most 32),
  `off` (one module).
- Scenarios per repetition, each with empty caches (own XO_BACKEND_CACHE_DIR, XO_BIN_CACHE and
  HOME): cold, warm (nothing changed), edit (one statement added to one function body of
  std/native/json.xo or compiler/ssa/passes.xo).
- Figures: median of 3 repetitions of `phase cc` (clang's compile and the link, ms) and wall
  seconds; modules reused from `cache llvm modules R/N`. Raw rows:
  llvm-test-split-2026-10-21.csv.
- Machine: Apple M4 Pro, 24 GB, macOS 27.0.1, other agents' jobs running (a bulk heavy.sh slot).

## Results

| Target | Mode | cold cc ms | edit cc ms (reused) | warm cc ms |
|---|---|---|---|---|
| json | 8 | 1129 | 610 (7/8) | 28 |
| json | files | 1982 | 352 (31/32) | 45 |
| json | off | 1560 | 1543 | 29 |
| ssa | 8 | 886 | 477 (7/8) | 29 |
| ssa | files | 1133 | 378 (31/32) | 27 |
| ssa | off | 1635 | 1638 | 24 |

## Decision

`files` is the default: it is the fastest edit build (100 to 260 ms less clang time than 8, about
1.2 s less than off), which is the build an agent repeats; it costs 250 to 850 ms more clang time
than 8 on a cold build, paid once per checkout. `8` and `off` stay available through
XO_LLVM_TEST_SPLIT.
