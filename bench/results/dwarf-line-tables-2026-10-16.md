# DWARF line tables: size, compile time, and checks (2026-10-16)

xo-fa, decision 0239. Measured on main ca769ce2 plus branch fa-dwarf.

## Size (bench/perf/tls, the TLS client)

| Platform, backend | Without (`-s`) | With | Growth |
|---|---|---|---|
| darwin/arm64, arm64 (selfhost/macho) | 2,384,497 | 2,384,495 executable + 596 KB PROG.dSYM | executable unchanged; dSYM 25% |
| linux/arm64, arm64 (elflink) | 2,369,896 | 2,965,728 | +25% |
| linux/arm64, LLVM | 2,456,696 | 3,406,224 (nameTableKind None; 3,570,168 with .debug_names) | +39% |

The decision estimated 10 to 25%. LLVM's line tables only units still
carry .debug_addr, .debug_rnglists, .debug_str_offsets, and the
subprograms of inlined functions.

## Compile time (darwin/arm64, bench/perf/tls, caches off, one run)

| | real | user |
|---|---|---|
| `-s` | 1.76 s | 3.98 s |
| default (debug) | 1.82 s | 4.22 s |

About +3% real and +6% user.

## Checks

- darwin: `dwarfdump --verify PROG.dSYM` clean; `dwarfdump --lookup`
  of main's address gives main.xo and its statement's line; `lldb -b -o
  "b main.xo:9" -o run` stops at main.xo:9:5 with the source shown
  (internal/arm64 TestDebugLineDarwin checks the dSYM, the lookup, and
  that `-s` writes none).
- linux/arm64 in Docker: arm64 with elflink and with `--ld=cc`:
  `llvm-dwarfdump --verify` clean, `--lookup` gives main.xo:9:5; LLVM
  with both links: llvm-symbolizer resolves through inlined frames to
  .xo lines; `--verify` reports only tail merged string offsets
  (gotchas/dwarf-verify-tail-merged-strings.md).
- Gates: internal/arm64 (goldens), devgoldens (arm64, LLVM, Go, ELF),
  self build then hello, fixed point, -short.
